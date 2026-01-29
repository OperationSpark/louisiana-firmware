## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## flash/mqttsensor: flash the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS]
.PHONY: flash/mqttsensor
TARGET ?= pico-w
flash/mqttsensor:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo flash -target=$(TARGET) -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./mqttsensor/...

## flash/mqttsensor/picow: flash the MQTT client app to the Pico W
## flash/mqttsensor/pico2w: flash the MQTT client app to the Pico2 W
.PHONY: flash/mqttsensor/picow flash/mqttsensor/pico2w
flash/mqttsensor/picow: TARGET=pico-w
flash/mqttsensor/picow: flash/mqttsensor
flash/mqttsensor/pico2w: TARGET=pico2-w
flash/mqttsensor/pico2w: flash/mqttsensor
