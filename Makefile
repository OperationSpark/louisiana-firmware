## help: print this help message
.PHONY: help
help:
	@echo 'Usage:'
	@sed -n 's/^##//p' ${MAKEFILE_LIST} | column -t -s ':' |  sed -e 's/^/ /'

## flash/mqttsensor: flash the MQTT client app to the Pico W. Pass env vars - MQTT_ADDR, WIFI_SSID, WIFI_PASS, [MQTT_USER, MQTT_PASS]
.PHONY: flash/mqttsensor
flash/mqttsensor:
	@LDFLAGS="-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.ssid=${WIFI_SSID}' \
		-X 'github.com/operationspark/aethernet-firmware/mqttsensor/cyw43439.pass=${WIFI_PASS}' \
		-X 'main.mqttServerAddr=${MQTT_ADDR}' \
		-X 'main.mqttUsername=${MQTT_USER}' \
		-X 'main.mqttPassword=${MQTT_PASS}'"; \
	tinygo flash -target=pico-w -stack-size=16kb -monitor -ldflags="$$LDFLAGS" ./mqttsensor/...

## flash/pinprobe: scan GP0..GP28 and report what's tied HIGH/LOW/floating.
.PHONY: flash/pinprobe
flash/pinprobe:
	tinygo flash -target=pico-w -monitor ./pinprobe/...
