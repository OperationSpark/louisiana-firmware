## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## flash/mqttsensor: flash the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
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

## flash/macprobe: print the Pico W's MAC address over serial. Use lowercased value as MQTT_DEVICE.
.PHONY: flash/macprobe
flash/macprobe:
	tinygo flash -target=pico-w -monitor ./macprobe/...

## flash/pinprobe: scan GP0..GP28 and report what's tied HIGH/LOW/floating.
.PHONY: flash/pinprobe
flash/pinprobe:
	tinygo flash -target=pico-w -monitor ./pinprobe/...
## build/mqttsensor: build the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
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

## build/sen0568mqtt: build the SEN0568 H2S (ADC0/GP26) + DHT11 MQTT app. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
.PHONY: build/sen0568mqtt
build/sen0568mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo build -target=pico-w -stack-size=16kb -ldflags="$$LDFLAGS" ./sen0568mqtt/...

## flash/sen0568mqtt: flash the SEN0568 H2S (ADC0/GP26) + DHT11 MQTT app. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
.PHONY: flash/sen0568mqtt
flash/sen0568mqtt:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttDeviceSlug=${MQTT_DEVICE}' \
		-X 'main.mqttTopic=${MQTT_TOPIC}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo flash -target=pico-w -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./sen0568mqtt/...

## flash/i2cscan: scan I2C devices on GP4/GP5.
.PHONY: flash/i2cscan
flash/i2cscan:
	tinygo flash -target=pico-w -monitor ./i2cscan/...

## build/sgp30mqtt: build the SGP30 + DHT11 MQTT app (no LCD, no MQ). Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
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

## flash/sgp30mqtt: flash the SGP30 + DHT11 MQTT app (no LCD, no MQ). Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, MQTT_USER, MQTT_DEVICE, [MQTT_PASS, MQTT_TOPIC]
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
