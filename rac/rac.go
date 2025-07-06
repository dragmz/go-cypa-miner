package rac

// #cgo windows LDFLAGS: -L${SRCDIR}/../rsagg/target/release -lrac -static -lNtosKrnl -lBCrypt
// #cgo linux   LDFLAGS: -L${SRCDIR}/../rsagg/target/release -lrac -static
// #include <stdarg.h>
// #include <stdint.h>
// #include <stdlib.h>
//
// typedef struct _Rac Rac;
//
// Rac *rac_new();
// void rac_free(Rac *c_rac);
// int rac_optimize(Rac *c_rac, const char *c_prefix, int time);
// typedef struct _RacSession RacSession;
// RacSession *rac_session_new(Rac *c_rac, const char *c_prefix, size_t batch_size);
// uint8_t* rac_session_result(RacSession *c_session);
// void rac_session_result_free(uint8_t *c_key);
// void rac_session_free(RacSession *c_session);
import "C"
import (
	"unsafe"

	"github.com/algorand/go-algorand-sdk/v2/crypto"
	"github.com/pkg/errors"
	"golang.org/x/crypto/ed25519"
)

type Rac struct {
	ptr *C.Rac
}

type RacSession struct {
	ptr *C.RacSession
}

func New() (*Rac, error) {
	ptr := C.rac_new()
	if ptr == nil {
		return nil, errors.New("failed to create Rac instance")
	}
	return &Rac{ptr: ptr}, nil
}

func (r *Rac) Free() {
	if r.ptr != nil {
		C.rac_free(r.ptr)
		r.ptr = nil
	}
}

func (r *Rac) Optimize(prefix string, time int) (uint64, error) {
	if r.ptr == nil {
		return 0, errors.New("Rac is not initialized")
	}

	cPrefix := C.CString(prefix)
	defer C.free(unsafe.Pointer(cPrefix))

	batch := C.rac_optimize(r.ptr, cPrefix, C.int(time))

	return uint64(batch), nil
}

func (r *Rac) NewSession(prefix string, batch uint64) (*RacSession, error) {
	if r.ptr == nil {
		return nil, errors.New("Rac is not initialized")
	}

	cPrefix := C.CString(prefix)
	defer C.free(unsafe.Pointer(cPrefix))

	session := C.rac_session_new(r.ptr, cPrefix, C.size_t(batch))
	if session == nil {
		return nil, errors.New("failed to create session")
	}

	return &RacSession{ptr: session}, nil
}

func (s *RacSession) Poll() (*crypto.Account, error) {
	ckey := (*C.uint8_t)(C.rac_session_result(s.ptr))
	if ckey == nil {
		return nil, nil
	}

	defer C.rac_session_result_free(ckey)

	sk := ed25519.NewKeyFromSeed([]byte(C.GoBytes(unsafe.Pointer(ckey), 32)))

	acc, err := crypto.AccountFromPrivateKey(sk)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create account from private key")
	}

	return &acc, nil
}

func (s *RacSession) Free() {
	if s.ptr != nil {
		C.rac_session_free(s.ptr)
		s.ptr = nil
	}
}
