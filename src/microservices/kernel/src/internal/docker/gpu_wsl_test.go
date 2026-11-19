package docker

import (
	"testing"

	"github.com/docker/docker/api/types/mount"
)

func TestSessionGPUDeviceRequests(t *testing.T) {
	if got := sessionGPUDeviceRequests(""); got != nil {
		t.Fatalf("GPU NVIDIA vide = %#v, attendu nil", got)
	}
	all := sessionGPUDeviceRequests("all")
	if len(all) != 1 || all[0].Driver != "nvidia" || all[0].Count != -1 {
		t.Fatalf("GPU NVIDIA all = %#v", all)
	}
	one := sessionGPUDeviceRequests("GPU-1234")
	if len(one) != 1 || len(one[0].DeviceIDs) != 1 || one[0].DeviceIDs[0] != "GPU-1234" {
		t.Fatalf("GPU NVIDIA ciblé = %#v", one)
	}
}

func TestSessionWSLGraphics(t *testing.T) {
	if got := sessionWSLGraphicsEnvironment("all", false); got != nil {
		t.Fatalf("environnement WSL sans dxg = %#v, attendu nil", got)
	}
	env := sessionWSLGraphicsEnvironment("all", true)
	if len(env) != 3 || env[0] != "GALLIUM_DRIVER=d3d12" ||
		env[1] != "MESA_D3D12_DEFAULT_ADAPTER_NAME=NVIDIA" {
		t.Fatalf("environnement WSL = %#v", env)
	}
	mounts := sessionWSLGraphicsMounts("all", true)
	if len(mounts) != 1 || mounts[0].Type != mount.TypeBind ||
		mounts[0].Source != "/usr/lib/wsl" || mounts[0].Target != "/usr/lib/wsl" || !mounts[0].ReadOnly {
		t.Fatalf("montage WSL = %#v", mounts)
	}
}
