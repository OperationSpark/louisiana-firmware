## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## sensor-test: flash the XIAO ESP32S3 sensor bring-up firmware, then attach the monitor.
## The ESP32-S3 re-enumerates its native USB on reset, so -monitor races the port. Reattach instead.
.PHONY: sensor-test
sensor-test:
	tinygo flash -target=xiao-esp32s3 -size=short ./cmd/sensor-test
	@sleep 3
	$(MAKE) monitor

## monitor: attach to the XIAO ESP32S3 serial monitor.
.PHONY: monitor
monitor:
	tinygo monitor -target=xiao-esp32s3

## build-sensor-test: compile the sensor bring-up firmware without flashing hardware.
.PHONY: build-sensor-test
build-sensor-test:
	@mkdir -p .build
	tinygo build -target=xiao-esp32s3 -size=short -o .build/sensor-test.elf ./cmd/sensor-test

## test: run host-side tests for hardware-independent logic.
.PHONY: test
test:
	go test ./internal/sensors

## flash/mqttsensor: flash the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC]
.PHONY: flash/mqttsensor
flash/mqttsensor:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo flash -target=pico-w -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./mqttsensor/...

## flash/pinprobe: scan GP0..GP28 and report what's tied HIGH/LOW/floating.
.PHONY: flash/pinprobe
flash/pinprobe:
	tinygo flash -target=pico-w -monitor ./pinprobe/...
## build/mqttsensor: build the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC]
.PHONY: build/mqttsensor
build/mqttsensor:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo build -target=pico-w -stack-size=16kb -ldflags="$$LDFLAGS" ./mqttsensor/...

## flash/i2cscan: scan I2C devices on GP4/GP5.
.PHONY: flash/i2cscan
flash/i2cscan:
	tinygo flash -target=pico-w -monitor ./i2cscan/...

## build/sgp30mqtt: build the SGP30 + DHT11 MQTT app (no LCD, no MQ). Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC]
.PHONY: build/sgp30mqtt
build/sgp30mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo build -target=pico-w -stack-size=16kb -ldflags="$$LDFLAGS" ./sgp30mqtt/...

## flash/sgp30mqtt: flash the SGP30 + DHT11 MQTT app (no LCD, no MQ). Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC]
.PHONY: flash/sgp30mqtt
flash/sgp30mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo flash -target=pico-w -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./sgp30mqtt/...

## build/so2mqtt: build the Alphasense SO2-A4 AFE MQTT app. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC, SO2_WEE_MV, SO2_AEE_MV, SO2_SENS_MV_PPB]
.PHONY: build/so2mqtt
build/so2mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}' \
		-X 'main.so2WEe=${SO2_WEE_MV}' \
		-X 'main.so2AEe=${SO2_AEE_MV}' \
		-X 'main.so2Sens=${SO2_SENS_MV_PPB}'"; \
	tinygo build -target=pico-w -stack-size=16kb -ldflags="$$LDFLAGS" ./so2mqtt/...

## flash/so2mqtt: flash the Alphasense SO2-A4 AFE MQTT app. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS, MQTT_DEVICE, MQTT_TOPIC, SO2_WEE_MV, SO2_AEE_MV, SO2_SENS_MV_PPB]
.PHONY: flash/so2mqtt
flash/so2mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}' \
		-X 'main.so2WEe=${SO2_WEE_MV}' \
		-X 'main.so2AEe=${SO2_AEE_MV}' \
		-X 'main.so2Sens=${SO2_SENS_MV_PPB}'"; \
	tinygo flash -target=pico-w -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./so2mqtt/...

## build/xiao-mqtt: build the XIAO ESP32-S3 WiFi+MQTT firmware (C++/Arduino, PlatformIO). Export WIFI_SSID, WIFI_PASS, MQTT_ADDR, MQTT_USER, MQTT_PASS, [MQTT_DEVICE, MQTT_TOPIC]
.PHONY: build/xiao-mqtt
build/xiao-mqtt:
	pio run -d esp32s3-mqtt

## flash/xiao-mqtt: flash the XIAO ESP32-S3 WiFi+MQTT firmware, then attach the monitor. Same env vars as build/xiao-mqtt.
.PHONY: flash/xiao-mqtt
flash/xiao-mqtt:
	pio run -d esp32s3-mqtt -t upload
	@sleep 3
	pio device monitor -d esp32s3-mqtt
