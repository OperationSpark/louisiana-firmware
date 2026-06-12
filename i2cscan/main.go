package main

import (
    "fmt"
    "machine"
    "time"
)

func main() {
    time.Sleep(2 * time.Second)
    
    i2c := machine.I2C0
    i2c.Configure(machine.I2CConfig{
        SCL:       machine.GP5,
        SDA:       machine.GP4,
        Frequency: 100000,
    })

    println("Scanning I2C bus...")
    for addr := uint8(1); addr < 128; addr++ {
        var buf [1]byte
        err := i2c.Tx(uint16(addr), nil, buf[:])
        if err == nil {
            fmt.Printf("Found device at 0x%X\n", addr)
        }
    }
    println("Done")
}
