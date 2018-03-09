// Copyright 2018 Jacques Supcik / HEIA-FR
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

// This package emulates a ws281x device and instead of driving RGB LEDs, it
// sends the corresponding array of numbers representing the color of the LEDs
// through a web socket. It is used to implement ws2811 web simulators.

package ws2811

import (
	"encoding/json"
	"time"

	"github.com/mohae/deepcopy"
	"github.com/pkg/errors"
)

const (
	// RpiPwmChannels is the number of PWM leds in the Raspberry Pi
	RpiPwmChannels = 2
	// TargetFreq is the target frequency. It is usually 800kHz (800000), and an go as low as 400000
	TargetFreq = 800000
	// DefaultLedCount is the default number of LEDs on the stripe.
	DefaultLedCount = 16
	// DefaultBrightness is the default maximum brightness of the LEDs. The brightness value can be between 0 and 255.
	DefaultBrightness = 64 // Safe value between 0 and 255.
)

// ChannelOption is the list of channel options
type ChannelOption struct {
	// LedCount is the number of LEDs, 0 if channel is unused
	LedCount int
	// StripeType is the strip color layout -- one of WS2811StripXXX constants
	StripeType int
	// Brightness is the maximum brightness of the LEDs. Value between 0 and 255
	Brightness int
	// WShift is the white shift value
	WShift int
	// RShift is the red shift value
	RShift int
	// GShift is the green shift value
	GShift int
	// BShift is blue shift value
	BShift int
	// Gamma is the gamma correction table
	Gamma []byte
}

// Option is the list of device options
type Option struct {
	// RenderWaitTime is the time in µs before the next render can run
	RenderWaitTime int
	// Frequency is the required output frequency
	Frequency int
	// Channels are channel options
	Channels []ChannelOption
}

// WS2811 represent the ws2811 device
type WS2811 struct {
	initialized bool
	options     *Option
	leds        [][]uint32
	hub         *Hub
	lastRender  time.Time
}

// DefaultOptions defines sensible default options for MakeWS2811
var DefaultOptions = Option{
	Frequency: TargetFreq,
	Channels: []ChannelOption{
		{
			LedCount:   DefaultLedCount,
			Brightness: DefaultBrightness,
			StripeType: WS2812Strip,
			Gamma:      nil,
		},
	},
}

// MakeWS2811 create an instance of web WS2811 linked to a given hub.
func MakeWS2811(opt *Option, hub *Hub) (ws2811 *WS2811, err error) {
	ws2811 = &WS2811{
		initialized: false,
	}
	ws2811.options = deepcopy.Copy(opt).(*Option)
	ws2811.hub = hub
	return ws2811, err
}

// Init initialize the device. It should be called only once before any other method.
func (ws2811 *WS2811) Init() error {
	if ws2811.initialized {
		return errors.New("device already initialized")
	}
	ws2811.leds = make([][]uint32, RpiPwmChannels)
	for i := 0; i < len(ws2811.options.Channels); i++ {
		ws2811.leds[i] = make([]uint32, ws2811.options.Channels[i].LedCount)
	}
	return nil
}

// Render sends a complete frame to the Web Socket
func (ws2811 *WS2811) Render() error {
	err := ws2811.Wait()
	if err != nil {
		return err
	}
	payload := struct {
		Option ChannelOption `json:"option"`
		Leds   []uint32      `json:"leds"`
	}{
		ws2811.options.Channels[0],
		ws2811.leds[0],
	}
	json, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	ws2811.hub.broadcast <- json
	ws2811.lastRender = time.Now()
	return err
}

// Wait waits for render to finish. The time needed for render is given by:
// time = 1/frequency * 8 * 3 * LedCount + 0.05
// (8 is the color depth and 3 is the number of colors (LEDs) per pixel).
// See https://cdn-shop.adafruit.com/datasheets/WS2811.pdf for more details.
func (ws2811 *WS2811) Wait() error {
	dt := (float64(8*3*ws2811.options.Channels[0].LedCount) + 0.05) / float64(ws2811.options.Frequency)
	nextRender := ws2811.lastRender.Add(time.Duration(dt * float64(time.Second)))
	time.Sleep(time.Until(nextRender))
	return nil
}

// Fini shuts down the device and frees memory.
func (ws2811 *WS2811) Fini() {
	ws2811.initialized = false
}

// Leds returns the LEDs array of a given channel
func (ws2811 *WS2811) Leds(channel int) []uint32 {
	return ws2811.leds[channel]
}

// SetLedsSync wait for the frame to finish and replace all the LEDs
func (ws2811 *WS2811) SetLedsSync(channel int, leds []uint32) error {
	if err := ws2811.Wait(); err != nil {
		return errors.WithMessage(err, "Error setting LEDs")
	}
	l := len(leds)
	if l > len(ws2811.leds[channel]) {
		return errors.New("Error: Too many LEDs")
	}
	for i := 0; i < l; i++ {
		ws2811.leds[channel][i] = leds[i]
	}
	return nil
}
