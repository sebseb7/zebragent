//go:build windows

package main

import (
	"context"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/registry"
)

const (
	globalReloadEventName = "Global\\ZebraAgentConfigReload"
	localReloadEventName  = "Local\\ZebraAgentConfigReload"
)

// createReloadEvent creates or opens an event with a permissive DACL so any session can signal it.
func createReloadEvent(name string) (windows.Handle, error) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return 0, err
	}

	// SDDL: D:(A;;GA;;;WD) -> Discretionary ACL: Allow Generic All to Everyone (WD)
	sd, err := windows.SecurityDescriptorFromString("D:(A;;GA;;;WD)")
	var sa *windows.SecurityAttributes
	if err == nil {
		sa = &windows.SecurityAttributes{
			Length:             uint32(unsafe.Sizeof(windows.SecurityAttributes{})),
			SecurityDescriptor: sd,
			InheritHandle:      0,
		}
	}

	h, err := windows.CreateEvent(sa, 0, 0, namePtr)
	if err != nil {
		h, err = windows.OpenEvent(windows.EVENT_ALL_ACCESS, false, namePtr)
	}
	return h, err
}

// startSignalWatcher sets up Win32 Named Event and Registry Change monitoring.
func startSignalWatcher(ctx context.Context, a *Agent) {
	go watchNamedEvents(ctx, a)
	go watchRegistryChanges(ctx, a)
}

func watchNamedEvents(ctx context.Context, a *Agent) {
	var handles []windows.Handle

	if hGlobal, err := createReloadEvent(globalReloadEventName); err == nil && hGlobal != 0 {
		handles = append(handles, hGlobal)
		logf("Listening for reload signal on Win32 event %s", globalReloadEventName)
	}
	if hLocal, err := createReloadEvent(localReloadEventName); err == nil && hLocal != 0 {
		handles = append(handles, hLocal)
		logf("Listening for reload signal on Win32 event %s", localReloadEventName)
	}

	if len(handles) == 0 {
		logf("Warning: Could not create Win32 reload event objects")
		return
	}
	defer func() {
		for _, h := range handles {
			windows.CloseHandle(h)
		}
	}()

	for {
		if ctx.Err() != nil {
			return
		}

		for _, h := range handles {
			res, _ := windows.WaitForSingleObject(h, 50)
			if res == windows.WAIT_OBJECT_0 {
				logf("Received Win32 event signal to reload configuration")
				_ = a.ReloadFromRegistry()
				time.Sleep(200 * time.Millisecond)
				break
			}
		}

		select {
		case <-ctx.Done():
			return
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func watchRegistryChanges(ctx context.Context, a *Agent) {
	type keyWatcher struct {
		root   registry.Key
		subkey string
	}
	watchers := []keyWatcher{
		{registry.CURRENT_USER, `Software\ZebraAgent`},
		{registry.LOCAL_MACHINE, `SOFTWARE\ZebraAgent`},
	}

	for _, w := range watchers {
		root := w.root
		subkey := w.subkey
		go func() {
			for {
				if ctx.Err() != nil {
					return
				}

				k, err := registry.OpenKey(root, subkey, registry.NOTIFY|registry.WOW64_64KEY)
				if err != nil {
					select {
					case <-ctx.Done():
						return
					case <-time.After(5 * time.Second):
						continue
					}
				}

				hEvent, err := windows.CreateEvent(nil, 0, 0, nil)
				if err != nil {
					k.Close()
					return
				}

				err = windows.RegNotifyChangeKeyValue(
					windows.Handle(k),
					false,
					windows.REG_NOTIFY_CHANGE_LAST_SET,
					hEvent,
					true,
				)
				if err != nil {
					windows.CloseHandle(hEvent)
					k.Close()
					return
				}

			waitLoop:
				for {
					if ctx.Err() != nil {
						windows.CloseHandle(hEvent)
						k.Close()
						return
					}
					res, _ := windows.WaitForSingleObject(hEvent, 500)
					if res == windows.WAIT_OBJECT_0 {
						time.Sleep(150 * time.Millisecond)
						logf("Detected registry change in %s; reloading configuration...", subkey)
						_ = a.ReloadFromRegistry()
						break waitLoop
					}
				}

				windows.CloseHandle(hEvent)
				k.Close()
			}
		}()
	}
}

