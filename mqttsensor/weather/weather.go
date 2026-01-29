// Package weather provides temperature and humidity sensing using the DHT11 sensor.
// It implements throttling and caching to handle the DHT11's minimum 2-second sampling interval.
package weather

import (
	"machine"
	"time"

	"tinygo.org/x/drivers/dht"
)

// Sensor wraps a DHT11 temperature and humidity sensor with throttling and caching.
// It automatically limits queries to respect the DHT11's minimum 2-second read interval.
type Sensor struct {
	dev             dht.Device           // Underlying DHT11 device driver.
	cachedTemp      float32              // Last successfully read temperature value.
	cachedHumidity  float32              // Last successfully read humidity value.
	lastReadTime    time.Time            // Timestamp of the last successful sensor read.
	minReadInterval time.Duration        // Minimum time required between sensor reads. Cached values are returned if subsequent read attempts are within the interval.
	hasValidCache   bool                 // Indicates whether cached values are available.
	tempScale       dht.TemperatureScale // Temperature scale for readings (Celsius or Fahrenheit).
}

func New(pin machine.Pin, scale dht.TemperatureScale) *Sensor {
	dev := dht.New(pin, dht.DHT11)
	return &Sensor{
		dev:           dev,
		hasValidCache: false,
		tempScale:     scale,
		// DHT11 requires minimum 2s between reads
		minReadInterval: 2 * time.Second,
	}
}

// ReadMeasurements reads temperature and humidity from the DHT11 sensor.
// Returns temperature, humidity, whether data is from cache, and any error.
// Uses throttling with cache: only queries the sensor if minReadInterval has passed
// since the last successful read. Returns cached values otherwise.
func (s *Sensor) ReadMeasurements() (temp float32, humidity float32, isCached bool, err error) {
	now := time.Now()

	// If we have a valid cache and not enough time has passed, return cached values
	if s.hasValidCache && now.Sub(s.lastReadTime) < s.minReadInterval {
		return s.cachedTemp, s.cachedHumidity, true, nil
	}

	// Enough time has passed (or no cache), read from sensor
	err = s.dev.ReadMeasurements()
	if err != nil {
		// If we have cached values, return them even on error
		if s.hasValidCache {
			return s.cachedTemp, s.cachedHumidity, true, err
		}
		// No cache, return zeros with error
		return 0, 0, false, err
	}

	// Get temperature in Celsius
	tempFloat, err := s.dev.TemperatureFloat(s.tempScale)
	if err != nil {
		if s.hasValidCache {
			return s.cachedTemp, s.cachedHumidity, true, err
		}
		return 0, 0, false, err
	}

	// Get relative humidity percentage
	humFloat, err := s.dev.HumidityFloat()
	if err != nil {
		if s.hasValidCache {
			return s.cachedTemp, s.cachedHumidity, true, err
		}
		return 0, 0, false, err
	}

	// Update cache with successful reading
	s.cachedTemp = tempFloat
	s.cachedHumidity = humFloat
	s.lastReadTime = now
	s.hasValidCache = true

	return tempFloat, humFloat, false, nil
}
