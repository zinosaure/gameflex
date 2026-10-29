// Command sdiag-guid prints the SDL2 controller GUID of the streamex virtual
// pad (Go replacement of the former Python helper). Dev-time diagnostic only:
// the GUID is deterministic (bus/vendor/product/name are constants) and is
// hardcoded in the entrypoint's SDL_GAMECONTROLLERCONFIG — nothing here runs
// during a game session.
package main

/*
#cgo pkg-config: sdl2
#include <SDL.h>
*/
import "C"

import (
	"fmt"
	"os"
	"strings"
	"unsafe"
)

func goStringOr(p *C.char) string {
	if p == nil {
		return ""
	}
	return C.GoString(p)
}

func main() {
	filter := "streamex pad"
	if len(os.Args) > 1 {
		filter = os.Args[1]
	}
	if C.SDL_Init(C.SDL_INIT_JOYSTICK|C.SDL_INIT_GAMECONTROLLER) != 0 {
		fmt.Printf("GUID=sdl-init-failed: %s\n", goStringOr(C.SDL_GetError()))
		os.Exit(1)
	}
	n := int(C.SDL_NumJoysticks())
	found := false
	for i := 0; i < n; i++ {
		var buf [34]C.char
		guid := C.SDL_JoystickGetDeviceGUID(C.int(i))
		C.SDL_JoystickGetGUIDString(guid, &buf[0], 33)
		name := ""
		if j := C.SDL_JoystickOpen(C.int(i)); j != nil {
			name = C.GoString(C.SDL_JoystickName(j))
		}
		if !strings.Contains(name, filter) {
			continue
		}
		found = true
		isGC := int(C.SDL_IsGameController(C.int(i)))
		mapping := ""
		if c := C.SDL_GameControllerOpen(C.int(i)); c != nil {
			mapping = goStringOr(C.SDL_GameControllerMapping(c))
		}
		fmt.Printf("GUID=%s name=%s is_gamecontroller=%d\n", C.GoString(&buf[0]), name, isGC)
		if mapping != "" {
			fmt.Printf("MAPPING=%s\n", mapping)
		}
	}
	if !found {
		fmt.Println("GUID=notfound")
	}
}

// silence "unsafe" unused warning if cgo not available on some builds
var _ = unsafe.Pointer(nil)
