// XIAO ESP32-S3 telemetry node.
//
// Reads a BME280 and an SGP41 over the STEMMA QT bus and publishes to the
// LouisianAir broker on the same topic and payload schema the Pico W firmware
// uses (see mqttsensor/mqtt/client.go). This exists because TinyGo has no WiFi
// support for any ESP32 part; see docs/esp32s3-mqtt-scope.md.

#include <Arduino.h>
#include <WiFi.h>
#include <Wire.h>
#include <PubSubClient.h>
#include <SensirionI2CSgp41.h>
#include <VOCGasIndexAlgorithm.h>
#include <NOxGasIndexAlgorithm.h>
#include <Adafruit_BME280.h>
#include <Preferences.h>
#include <time.h>

// XIAO D4/D5, matching internal/board/xiao_esp32s3.go.
static const int kSdaPin = 5;
static const int kSclPin = 6;
static const uint32_t kI2cFrequencyHz = 100000;

static const char *kTopicPrefix = "louisianair/dev";
// Sensirion's Gas Index Algorithm is a recursive filter: it assumes a fixed
// cadence and its NOx variant is hard-wired to 1 s (NOxGasIndexAlgorithm has no
// sampling-interval constructor, unlike the VOC one). So sample every second and
// publish every tenth sample rather than sampling every ten seconds.
static const uint32_t kSampleIntervalMs = 1000;
static const uint32_t kSamplesPerPublish = 10;

// Sensirion's own defaults, used to compensate the SGP41 until the BME280 has
// produced a reading (and for the whole run if no BME280 is attached).
static const float kDefaultTemperatureC = 25.0f;
static const float kDefaultHumidityPct = 50.0f;

// The SGP41 heater needs 10 seconds of conditioning before its first real
// measurement; readings taken earlier are meaningless, not merely noisy.
static const int kConditioningSeconds = 10;

// The algorithm returns 0 for its first GasIndexAlgorithm_INITIAL_BLACKOUT (45)
// seconds, and its VOC state is only meaningful after 3 hours of continuous
// operation. Sensirion also documents that restored state must not be used after
// an interruption longer than 10 minutes.
static const uint32_t kStateMinRuntimeMs = 3UL * 3600UL * 1000UL;
static const uint32_t kStateSaveIntervalMs = 3600UL * 1000UL;
static const uint32_t kStateMaxAgeSeconds = 600;

static SensirionI2CSgp41 sgp41;
// Default constructors mean the 1 s sampling interval both algorithms expect.
static VOCGasIndexAlgorithm vocAlgorithm;
static NOxGasIndexAlgorithm noxAlgorithm;
static Adafruit_BME280 bme;
static bool bmePresent = false;
static Preferences prefs;

static WiFiClient wifiClient;
static PubSubClient mqtt(wifiClient);

static char deviceSlug[32];
static char topic[160];
static char mqttHost[64];
static uint16_t mqttPort = 1883;

// formatMac writes the lowercase colon-separated form, matching esptool and the
// boot line printed by cmd/sensor-test.
static void formatMac(const uint8_t mac[6], char *out, size_t len) {
  snprintf(out, len, "%02x:%02x:%02x:%02x:%02x:%02x", mac[0], mac[1], mac[2],
           mac[3], mac[4], mac[5]);
}

// splitAddr parses the "host:port" form used by MQTT_ADDR in the Makefile. A
// missing port keeps the 1883 default rather than failing the build config.
static bool splitAddr(const char *addr, char *host, size_t hostLen,
                      uint16_t *port) {
  if (addr == nullptr || addr[0] == '\0') {
    return false;
  }
  const char *colon = strrchr(addr, ':');
  if (colon == nullptr) {
    snprintf(host, hostLen, "%s", addr);
    return true;
  }
  size_t n = (size_t)(colon - addr);
  if (n == 0 || n >= hostLen) {
    return false;
  }
  memcpy(host, addr, n);
  host[n] = '\0';
  long parsed = strtol(colon + 1, nullptr, 10);
  if (parsed <= 0 || parsed > 65535) {
    return false;
  }
  *port = (uint16_t)parsed;
  return true;
}

// buildPayload renders the canonical LouisianAir telemetry document. Absent
// hardware is omitted rather than sent as an assumed value, so downstream never
// mistakes a compensation default for a measurement.
//
// Kept free of globals so selfCheck can exercise it with fixed inputs.
static int buildPayload(char *out, size_t len, const char *slug,
                        uint64_t recordedAtUnixNs, uint64_t sinceBootNs,
                        bool haveEnv, float temperatureC, float humidityPct,
                        float pressureHPa, bool haveGas, uint16_t srawVoc,
                        uint16_t srawNox, bool haveIndex, int32_t vocIndex,
                        int32_t noxIndex) {
  time_t seconds = (time_t)(recordedAtUnixNs / 1000000000ULL);
  struct tm utc;
  gmtime_r(&seconds, &utc);
  char recordedAt[32];
  strftime(recordedAt, sizeof(recordedAt), "%Y-%m-%dT%H:%M:%SZ", &utc);

  char measurements[256];
  int m = 0;
  measurements[0] = '\0';
  if (haveEnv) {
    m += snprintf(measurements + m, sizeof(measurements) - m,
                  "\"temperature\":%.2f,\"humidity\":%.2f,\"pressure\":%.2f",
                  temperatureC, humidityPct, pressureHPa);
  }
  if (haveGas) {
    m += snprintf(measurements + m, sizeof(measurements) - m,
                  "%s\"sraw_voc\":%u,\"sraw_nox\":%u", m > 0 ? "," : "",
                  srawVoc, srawNox);
  }
  // The raw ticks ride along with the indices on purpose: they are what the
  // algorithm consumes, so keeping them lets an index be recomputed offline if
  // the tuning parameters ever change.
  if (haveIndex) {
    m += snprintf(measurements + m, sizeof(measurements) - m,
                  "%s\"sensirion_voc_index\":%ld,\"sensirion_nox_index\":%ld",
                  m > 0 ? "," : "", (long)vocIndex, (long)noxIndex);
  }

  return snprintf(out, len,
                  "{\"source_reading_key\":\"%s:%llu:%llu\",\"recorded_at\":"
                  "\"%s\",\"measurements\":{%s}}",
                  slug, (unsigned long long)recordedAtUnixNs,
                  (unsigned long long)sinceBootNs, recordedAt, measurements);
}

// selfCheck exercises the two pieces of logic that are wrong-by-a-character
// rather than wrong-by-a-wire: address splitting and payload rendering. It runs
// on every boot because a node that cannot format its own telemetry should say
// so on the serial line instead of publishing garbage for a week.
static void selfCheck() {
  char host[64];
  uint16_t port = 1883;
  bool ok = splitAddr("broker.example.org:8883", host, sizeof(host), &port);
  if (!ok || strcmp(host, "broker.example.org") != 0 || port != 8883) {
    Serial.println("[CHECK] FAIL: splitAddr host:port");
    return;
  }
  port = 1883;
  ok = splitAddr("10.0.0.9", host, sizeof(host), &port);
  if (!ok || strcmp(host, "10.0.0.9") != 0 || port != 1883) {
    Serial.println("[CHECK] FAIL: splitAddr bare host");
    return;
  }

  char payload[384];
  buildPayload(payload, sizeof(payload), "node-a", 1757332800000000000ULL,
               1500000000ULL, true, 21.5f, 44.25f, 1013.0f, true, 30000, 17000,
               true, 103, 1);
  const char *want =
      "{\"source_reading_key\":\"node-a:1757332800000000000:1500000000\","
      "\"recorded_at\":\"2025-09-08T12:00:00Z\",\"measurements\":{"
      "\"temperature\":21.50,\"humidity\":44.25,\"pressure\":1013.00,"
      "\"sraw_voc\":30000,\"sraw_nox\":17000,"
      "\"sensirion_voc_index\":103,\"sensirion_nox_index\":1}}";
  if (strcmp(payload, want) != 0) {
    Serial.println("[CHECK] FAIL: payload mismatch");
    Serial.print("[CHECK] got:  ");
    Serial.println(payload);
    Serial.print("[CHECK] want: ");
    Serial.println(want);
    return;
  }

  // Gas-only: the comma between groups must not appear when the first is empty.
  buildPayload(payload, sizeof(payload), "n", 1757332800000000000ULL, 1ULL,
               false, 0, 0, 0, true, 1, 2, false, 0, 0);
  if (strstr(payload, "{\"sraw_voc\":1") == nullptr) {
    Serial.println("[CHECK] FAIL: gas-only payload has a stray separator");
    return;
  }
  // A blacked-out index must be absent, not published as a literal 0: the
  // backend would store that as a real reading of "cleanest possible air".
  if (strstr(payload, "sensirion_voc_index") != nullptr) {
    Serial.println("[CHECK] FAIL: index emitted while unavailable");
    return;
  }
  Serial.println("[CHECK] ok");
}

// The VOC algorithm spends its first hours learning a baseline, so a reboot
// otherwise throws away everything it knows about this location's normal air.
// Only the VOC algorithm exposes state: NOxGasIndexAlgorithm declares no
// get_states/set_states, so NOx always relearns from scratch after a restart.
//
// Restoring is only valid for a short interruption, so the save timestamp is
// stored alongside the state and checked against the (NTP-synced) clock.
static void restoreVocState() {
  prefs.begin("sgp41", true);
  uint32_t savedAt = prefs.getUInt("saved_at", 0);
  float state0 = prefs.getFloat("voc0", 0.0f);
  float state1 = prefs.getFloat("voc1", 0.0f);
  prefs.end();

  if (savedAt == 0) {
    Serial.println("[SGP41] no saved VOC state; starting initial learning");
    return;
  }
  uint32_t now = (uint32_t)time(nullptr);
  if (now < savedAt || now - savedAt > kStateMaxAgeSeconds) {
    Serial.printf("[SGP41] saved VOC state is %lus old (limit %lus); starting "
                  "initial learning\n",
                  (unsigned long)(now - savedAt),
                  (unsigned long)kStateMaxAgeSeconds);
    return;
  }
  vocAlgorithm.set_states(state0, state1);
  Serial.printf("[SGP41] restored VOC state saved %lus ago\n",
                (unsigned long)(now - savedAt));
}

static void saveVocState() {
  float state0 = 0.0f, state1 = 0.0f;
  vocAlgorithm.get_states(state0, state1);
  prefs.begin("sgp41", false);
  prefs.putFloat("voc0", state0);
  prefs.putFloat("voc1", state1);
  prefs.putUInt("saved_at", (uint32_t)time(nullptr));
  prefs.end();
  Serial.println("[SGP41] saved VOC state");
}

static void connectWifi() {
  Serial.printf("[WIFI] joining %s\n", WIFI_SSID);
  WiFi.mode(WIFI_STA);
  WiFi.begin(WIFI_SSID, WIFI_PASS);
  while (WiFi.status() != WL_CONNECTED) {
    delay(500);
    Serial.print(".");
  }
  Serial.println();
  Serial.printf("[WIFI] connected ip=%s rssi=%d\n",
                WiFi.localIP().toString().c_str(), WiFi.RSSI());
}

// syncTime blocks until NTP lands. The canonical payload requires a real
// timestamp, so publishing before the clock is set would emit 1970 keys that
// collide across every node on the network.
static void syncTime() {
  configTime(0, 0, "pool.ntp.org", "time.nist.gov");
  Serial.print("[NTP] syncing");
  time_t now = 0;
  while (now < 1700000000) {
    delay(500);
    Serial.print(".");
    now = time(nullptr);
  }
  Serial.println();
  Serial.printf("[NTP] synced unix=%lld\n", (long long)now);
}

static void connectMqtt() {
  while (!mqtt.connected()) {
    Serial.printf("[MQTT] connecting to %s:%u as %s\n", mqttHost, mqttPort,
                  deviceSlug);
    bool ok;
    if (strlen(MQTT_USER) > 0) {
      ok = mqtt.connect(deviceSlug, MQTT_USER, MQTT_PASS);
    } else {
      ok = mqtt.connect(deviceSlug);
    }
    if (ok) {
      Serial.printf("[MQTT] connected topic=%s\n", topic);
      return;
    }
    Serial.printf("[MQTT] connect failed state=%d; retrying in 5s\n",
                  mqtt.state());
    delay(5000);
  }
}

void setup() {
  Serial.begin(115200);
  // Native USB has to enumerate before anything printed here is visible.
  delay(3000);

  Serial.println("[BOOT] XIAO ESP32-S3 telemetry node");

  uint8_t mac[6];
  WiFi.macAddress(mac);
  char macStr[18];
  formatMac(mac, macStr, sizeof(macStr));

  if (strlen(MQTT_DEVICE) > 0) {
    snprintf(deviceSlug, sizeof(deviceSlug), "%s", MQTT_DEVICE);
  } else {
    snprintf(deviceSlug, sizeof(deviceSlug), "xiao-%02x%02x%02x%02x%02x%02x",
             mac[0], mac[1], mac[2], mac[3], mac[4], mac[5]);
  }
  Serial.printf("[BOOT] mac=%s device=%s\n", macStr, deviceSlug);

  selfCheck();

  if (!splitAddr(MQTT_ADDR, mqttHost, sizeof(mqttHost), &mqttPort)) {
    Serial.println("[MQTT] ERROR: MQTT_ADDR is empty or malformed; export it "
                   "as host:port and reflash");
    while (true) {
      delay(1000);
    }
  }

  if (strlen(MQTT_TOPIC) > 0) {
    snprintf(topic, sizeof(topic), "%s", MQTT_TOPIC);
  } else if (strlen(MQTT_USER) > 0) {
    snprintf(topic, sizeof(topic), "%s/%s/%s/telemetry", kTopicPrefix,
             MQTT_USER, deviceSlug);
  } else {
    Serial.println("[MQTT] ERROR: set MQTT_USER (for the default topic) or "
                   "MQTT_TOPIC and reflash");
    while (true) {
      delay(1000);
    }
  }

  Wire.begin(kSdaPin, kSclPin, kI2cFrequencyHz);

  sgp41.begin(Wire);
  uint16_t testResult = 0;
  if (sgp41.executeSelfTest(testResult) != 0 || testResult != 0xD400) {
    Serial.printf("[SGP41] WARN: self test result=0x%04x\n", testResult);
  }

  // 0x76 first, then 0x77: both are shipped as defaults by different breakouts.
  bmePresent = bme.begin(0x76, &Wire) || bme.begin(0x77, &Wire);
  if (!bmePresent) {
    Serial.println("[BME280] WARN: not found; publishing gas readings only and "
                   "compensating the SGP41 with 25C/50%RH");
  }

  connectWifi();
  syncTime();

  mqtt.setServer(mqttHost, mqttPort);
  // The default 256-byte buffer truncates a full payload silently.
  mqtt.setBufferSize(512);
  connectMqtt();

  Serial.println("[SGP41] conditioning");
  for (int second = 0; second < kConditioningSeconds; second++) {
    uint16_t srawVoc = 0;
    sgp41.executeConditioning(0x8000, 0x6666, srawVoc);
    delay(1000);
  }
  Serial.println("[SGP41] ready");

  // After NTP, so the age check has a real clock to compare against.
  restoreVocState();
  Serial.println("[APP] sampling at 1 Hz, publishing every 10th sample");
}

void loop() {
  // A steady cadence is not cosmetic here: the Gas Index Algorithm is a
  // recursive filter whose time constants assume one sample per second.
  static uint32_t nextSampleMs = 0;
  static uint32_t sampleCount = 0;
  static uint32_t lastStateSaveMs = 0;

  if ((int32_t)(millis() - nextSampleMs) < 0) {
    mqtt.loop();
    delay(5);
    return;
  }
  nextSampleMs += kSampleIntervalMs;
  // A blocking reconnect or a stalled publish can push the deadline into the
  // past; resync rather than firing a burst of catch-up samples the algorithm
  // would read as a minute of real air.
  if ((int32_t)(millis() - nextSampleMs) > (int32_t)kSampleIntervalMs) {
    nextSampleMs = millis() + kSampleIntervalMs;
  }

  if (WiFi.status() != WL_CONNECTED) {
    connectWifi();
  }
  if (!mqtt.connected()) {
    connectMqtt();
  }
  mqtt.loop();

  float temperatureC = kDefaultTemperatureC;
  float humidityPct = kDefaultHumidityPct;
  float pressureHPa = 0;
  bool haveEnv = false;
  if (bmePresent) {
    temperatureC = bme.readTemperature();
    humidityPct = bme.readHumidity();
    pressureHPa = bme.readPressure() / 100.0f;
    haveEnv = !isnan(temperatureC) && !isnan(humidityPct);
  }

  // Sensirion's tick encoding: RH over 0..65535 for 0..100%, temperature over
  // the same range for -45..130C.
  uint16_t rhTicks = (uint16_t)(humidityPct * 65535.0f / 100.0f);
  uint16_t tempTicks = (uint16_t)((temperatureC + 45.0f) * 65535.0f / 175.0f);
  uint16_t srawVoc = 0, srawNox = 0;
  bool haveGas =
      sgp41.measureRawSignals(rhTicks, tempTicks, srawVoc, srawNox) == 0;

  // process() returns 0 through the algorithm's 45-second initial blackout, and
  // the NOx index stays untrustworthy far longer than that (its mean and
  // variance initialize over ~4.75 h and ~5.70 h). Zero is a sentinel, not a
  // measurement, so it is omitted rather than published.
  int32_t vocIndex = 0, noxIndex = 0;
  if (haveGas) {
    vocIndex = vocAlgorithm.process((int32_t)srawVoc);
    noxIndex = noxAlgorithm.process((int32_t)srawNox);
  } else {
    Serial.println("[SGP41] ERROR: measurement failed; retrying next second");
  }
  bool haveIndex = haveGas && vocIndex > 0 && noxIndex > 0;

  // Saved state is only meaningful once the algorithm has run long enough for
  // its baseline to mean something.
  uint32_t uptimeMs = millis();
  if (uptimeMs >= kStateMinRuntimeMs &&
      uptimeMs - lastStateSaveMs >= kStateSaveIntervalMs) {
    saveVocState();
    lastStateSaveMs = uptimeMs;
  }

  if (++sampleCount % kSamplesPerPublish != 0) {
    return;
  }

  struct timespec ts;
  clock_gettime(CLOCK_REALTIME, &ts);
  uint64_t recordedAtUnixNs =
      (uint64_t)ts.tv_sec * 1000000000ULL + (uint64_t)ts.tv_nsec;
  uint64_t sinceBootNs = (uint64_t)esp_timer_get_time() * 1000ULL;

  char payload[448];
  int written = buildPayload(payload, sizeof(payload), deviceSlug,
                             recordedAtUnixNs, sinceBootNs, haveEnv,
                             temperatureC, humidityPct, pressureHPa, haveGas,
                             srawVoc, srawNox, haveIndex, vocIndex, noxIndex);
  if (written <= 0 || written >= (int)sizeof(payload)) {
    Serial.println("[MQTT] ERROR: payload did not fit; skipping publish");
  } else if (mqtt.publish(topic, payload)) {
    Serial.printf("[MQTT] published %s\n", payload);
  } else {
    Serial.println("[MQTT] ERROR: publish failed");
  }
}
