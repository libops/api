package types

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
)

type RawJSON json.RawMessage

func (j *RawJSON) Scan(value interface{}) error {
	if value == nil {
		*j = nil
		return nil
	}
	bytes, ok := value.([]byte)
	if !ok {
		return errors.New("type assertion to []byte failed")
	}
	*j = append((*j)[0:0], bytes...)
	return nil
}

func (j RawJSON) Value() (driver.Value, error) {
	if j == nil {
		return nil, nil
	}
	return []byte(j), nil
}

func (j RawJSON) MarshalJSON() ([]byte, error) {
	if j == nil {
		return []byte("null"), nil
	}
	return json.RawMessage(j).MarshalJSON()
}

func (j *RawJSON) UnmarshalJSON(data []byte) error {
	if j == nil {
		return errors.New("json: UnmarshalJSON on nil pointer")
	}
	*j = append((*j)[0:0], data...)
	return nil
}
