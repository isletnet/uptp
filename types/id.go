package types

import (
	"encoding/json"
	"fmt"
)

// ID 自定义ID类型，用于处理大数值ID的JSON序列化
type ID uint64

// MarshalJSON 实现 json.Marshaler 接口
func (id ID) MarshalJSON() ([]byte, error) {
	return json.Marshal(id.String())
}

// UnmarshalJSON 实现 json.Unmarshaler 接口
func (id *ID) UnmarshalJSON(data []byte) error {
	var s string
	if err := json.Unmarshal(data, &s); err != nil {
		return err
	}
	var v uint64
	_, err := fmt.Sscanf(s, "%d", &v)
	if err != nil {
		return err
	}
	*id = ID(v)
	return nil
}

// String 实现 fmt.Stringer 接口
func (id ID) String() string {
	return fmt.Sprintf("%d", uint64(id))
}

// Uint64 转换为 uint64
func (id ID) Uint64() uint64 {
	return uint64(id)
}
