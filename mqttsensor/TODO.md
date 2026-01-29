# TODO

## Features to Add

- [x] Add NTP on startup to get UTC time available for measurements
  - Nice to have: Check for epoch year 2035 issue
- [x] Connect to MQTT broker and publish sensor data

  - [x] Unauthenticated connection
  - [ ] Authenticated connection (username/password)
  - [ ] Support for multiple sensors
  - [ ] Configurable MQTT topics

- [ ] Add code to smooth sensor readings

  - [ ] 8-32 bit moving average filter?

- [ ] Create a display 'layers' so that a user can toggle between different sets of information on the LCD
  - [ ] For example: MQTT connection state, IP address, sensor readings, etc.
  - [ ] Display startup and error messages as the program loads (after LCD is initialized)
- [ ] HTTP server for configuration and status monitoring ?
  - [ ] Web interface to configure WiFi, MQTT, and sensor settings
  - [ ] Display current sensor readings and connection status
- [ ] Persist to SD card ?
  - [ ] Log sensor data with timestamps
  - [ ] Store configuration settings
