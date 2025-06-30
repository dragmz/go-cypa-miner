package main

import "C"
import (
	"fmt"
	"gvm"
	"runtime/cgo"
	"unsafe"
)

//export NewMiner
func NewMiner(c_algod *C.char, c_algodToken *C.char, appid uint64) uintptr {
	algod := C.GoString(c_algod)
	algodToken := C.GoString(c_algodToken)

	m, err := gvm.NewMiner(algod, algodToken, appid)
	if err != nil {
		fmt.Printf("Error creating miner: %v\n", err)
		return 0
	}

	handle := cgo.NewHandle(m)
	return uintptr(handle)
}

//export FreeMiner
func FreeMiner(handle uintptr) {
	h := cgo.Handle(handle)
	h.Delete()
}

//export Fulfill
func Fulfill(handle uintptr, c_sk *C.char, key_len C.int, c_key *C.char, c_owner *C.char) {
	sk := C.GoBytes(unsafe.Pointer(c_sk), 64)
	key := C.GoBytes(unsafe.Pointer(c_key), key_len)
	owner := C.GoBytes(unsafe.Pointer(c_owner), 32)

	m := cgo.Handle(handle).Value().(*gvm.Miner)
	if m == nil {
		fmt.Println("Miner handle is nil")
		return
	}
	err := m.Fulfill(sk, key, owner)
	if err != nil {
		fmt.Printf("Error fulfilling: %v\n", err)
		return
	}
}

func main() {}
