package platform

import (
	"context"
	"errors"
	"unsafe"

	"golang.org/x/sys/windows"
)

type Credentials struct{}

func NewCredentials(string) *Credentials { return &Credentials{} }

func (*Credentials) Seal(_ context.Context, id string, plaintext []byte) ([]byte, error) {
	return dpapi(id, plaintext, true)
}

func (*Credentials) Open(_ context.Context, id string, ciphertext []byte) ([]byte, error) {
	return dpapi(id, ciphertext, false)
}

func dpapi(id string, value []byte, seal bool) ([]byte, error) {
	if len(value) == 0 || id == "" {
		return nil, errors.New("Ungültige Zugangsdaten.")
	}
	entropy := []byte(id)
	binding := windows.DataBlob{Size: uint32(len(entropy)), Data: &entropy[0]}
	input := windows.DataBlob{Size: uint32(len(value)), Data: &value[0]}
	var output windows.DataBlob
	var err error
	if seal {
		err = windows.CryptProtectData(&input, nil, &binding, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	} else {
		err = windows.CryptUnprotectData(&input, nil, &binding, 0, nil, windows.CRYPTPROTECT_UI_FORBIDDEN, &output)
	}
	if err != nil {
		return nil, errors.New("DPAPI-Zugriff fehlgeschlagen. Windows-Ausführungskonto prüfen.")
	}
	defer windows.LocalFree(windows.Handle(uintptr(unsafe.Pointer(output.Data))))
	data := unsafe.Slice(output.Data, output.Size)
	result := append([]byte(nil), data...)
	clear(data)
	return result, nil
}
