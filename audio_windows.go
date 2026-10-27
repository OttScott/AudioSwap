package main

import (
	"errors"
	"fmt"
	"runtime"
	"syscall"
	"unsafe"

	"github.com/go-ole/go-ole"
	"github.com/moutend/go-wca/pkg/wca"
)

var (
	policyClass = ole.NewGUID("{870AF99C-171D-4F9E-AF0D-E63DF40C2BC9}")
	policyIID   = ole.NewGUID("{F8679F50-850A-41CF-9C72-430F290290C8}")
	clearProp   = syscall.NewLazyDLL("ole32.dll").NewProc("PropVariantClear")
)

// IPolicyConfig is undocumented; SetDefaultEndpoint follows ten policy methods.
// Interface layout: https://github.com/tartakynov/audioswitch/blob/master/IPolicyConfig.h
type policyConfig struct {
	ole.IUnknown
}

type policyConfigVTable struct {
	ole.IUnknownVtbl
	GetMixFormat          uintptr
	GetDeviceFormat       uintptr
	ResetDeviceFormat     uintptr
	SetDeviceFormat       uintptr
	GetProcessingPeriod   uintptr
	SetProcessingPeriod   uintptr
	GetShareMode          uintptr
	SetShareMode          uintptr
	GetPropertyValue      uintptr
	SetPropertyValue      uintptr
	SetDefaultEndpoint    uintptr
	SetEndpointVisibility uintptr
}

type windowsAudio struct {
	enumerator *wca.IMMDeviceEnumerator
	policy     *policyConfig
}

func openAudio() (audioClient, error) {
	runtime.LockOSThread()
	err := ole.CoInitializeEx(0, ole.COINIT_APARTMENTTHREADED)
	var oleErr *ole.OleError
	if err != nil && !(errors.As(err, &oleErr) && oleErr.Code() == 1) {
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("initialize COM: %w", err)
	}
	var enumerator *wca.IMMDeviceEnumerator
	if err := wca.CoCreateInstance(wca.CLSID_MMDeviceEnumerator, 0, ole.CLSCTX_ALL, wca.IID_IMMDeviceEnumerator, &enumerator); err != nil {
		ole.CoUninitialize()
		runtime.UnlockOSThread()
		return nil, fmt.Errorf("create audio enumerator: %w", err)
	}
	return &windowsAudio{enumerator: enumerator}, nil
}

func (a *windowsAudio) Close() {
	if a.policy != nil {
		a.policy.Release()
	}
	a.enumerator.Release()
	ole.CoUninitialize()
	runtime.UnlockOSThread()
}

func (a *windowsAudio) Devices() ([]device, error) {
	var collection *wca.IMMDeviceCollection
	// Removed adapters can leave historical endpoints with unreadable properties.
	states := uint32(wca.DEVICE_STATE_ACTIVE | wca.DEVICE_STATE_DISABLED | wca.DEVICE_STATE_UNPLUGGED)
	if err := a.enumerator.EnumAudioEndpoints(wca.ERender, states, &collection); err != nil {
		return nil, err
	}
	defer collection.Release()
	var count uint32
	if err := collection.GetCount(&count); err != nil {
		return nil, err
	}
	devices := make([]device, 0, count)
	for i := uint32(0); i < count; i++ {
		d, err := readDevice(collection, i)
		if err != nil {
			return nil, fmt.Errorf("read output %d: %w", i+1, err)
		}
		devices = append(devices, d)
	}
	return devices, nil
}

func readDevice(collection *wca.IMMDeviceCollection, index uint32) (device, error) {
	var endpoint *wca.IMMDevice
	if err := collection.Item(index, &endpoint); err != nil {
		return device{}, err
	}
	defer endpoint.Release()
	var d device
	if err := endpoint.GetId(&d.ID); err != nil {
		return device{}, fmt.Errorf("read endpoint ID: %w", err)
	}
	var state uint32
	if err := endpoint.GetState(&state); err != nil {
		return device{}, fmt.Errorf("read state for %q: %w", d.ID, err)
	}
	d.Active = state&wca.DEVICE_STATE_ACTIVE != 0
	switch state {
	case wca.DEVICE_STATE_ACTIVE:
		d.State = "active"
	case wca.DEVICE_STATE_DISABLED:
		d.State = "disabled"
	case wca.DEVICE_STATE_NOTPRESENT:
		d.State = "not present"
	case wca.DEVICE_STATE_UNPLUGGED:
		d.State = "unplugged"
	default:
		d.State = fmt.Sprintf("0x%x", state)
	}
	var properties *wca.IPropertyStore
	if err := endpoint.OpenPropertyStore(wca.STGM_READ, &properties); err != nil {
		return device{}, fmt.Errorf("open properties for %q (%s): %w", d.ID, d.State, err)
	}
	defer properties.Release()
	var value wca.PROPVARIANT
	if err := properties.GetValue(&wca.PKEY_Device_FriendlyName, &value); err != nil {
		return device{}, fmt.Errorf("read friendly name for %q (%s): %w", d.ID, d.State, err)
	}
	if value.VT != ole.VT_LPWSTR {
		err := fmt.Errorf("device name has unexpected property type %d", value.VT)
		hr, _, _ := clearProp.Call(uintptr(unsafe.Pointer(&value)))
		if int32(hr) < 0 {
			err = errors.Join(err, fmt.Errorf("free device name: %w", ole.NewError(hr)))
		}
		return device{}, err
	}
	// go-wca's String copies and frees the LPWSTR buffer.
	d.Name = value.String()
	if d.Name == "" {
		return device{}, fmt.Errorf("output %q has no friendly name", d.ID)
	}
	return d, nil
}

func (a *windowsAudio) Default(r role) (string, error) {
	var endpoint *wca.IMMDevice
	err := a.enumerator.GetDefaultAudioEndpoint(wca.ERender, uint32(r), &endpoint)
	var oleErr *ole.OleError
	// E_NOTFOUND means Windows has no default for this role, not an API failure.
	if errors.As(err, &oleErr) && uint32(oleErr.Code()) == 0x80070490 {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	defer endpoint.Release()
	var id string
	if err := endpoint.GetId(&id); err != nil {
		return "", err
	}
	return id, nil
}

func (a *windowsAudio) SetDefault(id string, r role) error {
	if a.policy == nil {
		if err := wca.CoCreateInstance(policyClass, 0, ole.CLSCTX_ALL, policyIID, &a.policy); err != nil {
			return fmt.Errorf("create Windows audio policy client: %w", err)
		}
	}
	wideID, err := syscall.UTF16PtrFromString(id)
	if err != nil {
		return err
	}
	table := (*policyConfigVTable)(unsafe.Pointer(a.policy.RawVTable))
	hr, _, _ := syscall.SyscallN(
		table.SetDefaultEndpoint,
		uintptr(unsafe.Pointer(a.policy)),
		uintptr(unsafe.Pointer(wideID)),
		uintptr(r),
	)
	runtime.KeepAlive(wideID)
	if int32(hr) < 0 {
		return ole.NewError(hr)
	}
	current, err := a.Default(r)
	if err != nil {
		return fmt.Errorf("verify changed default: %w", err)
	}
	if current != id {
		return fmt.Errorf("Windows reported success but the default is %q, not %q", current, id)
	}
	return nil
}
