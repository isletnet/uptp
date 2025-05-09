// Time-based One-time Password
package totp

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/binary"
)

const (
	StepShareValidityPeriod = 300
	StepRefreshToken        = 60 * 60 * 24 * 7
)

type TOTP struct {
	Step int64
}

func (t *TOTP) Gen(token uint64, ts int64) uint64 {
	step := ts / t.Step
	tbuff := make([]byte, 8)
	binary.LittleEndian.PutUint64(tbuff, token)
	mac := hmac.New(sha256.New, tbuff)
	b := make([]byte, 8)
	binary.LittleEndian.PutUint64(b, uint64(step))
	mac.Write(b)
	num := binary.LittleEndian.Uint64(mac.Sum(nil)[:8])
	// fmt.Printf("%x\n", mac.Sum(nil))
	return num
}

func (t *TOTP) Verify(code uint64, token uint64, ts int64) bool {
	if code == 0 {
		return false
	}
	if code == token {
		return true
	}
	if code == t.Gen(token, ts) || code == t.Gen(token, ts-t.Step) || code == t.Gen(token, ts+t.Step) {
		return true
	}
	return false
}
