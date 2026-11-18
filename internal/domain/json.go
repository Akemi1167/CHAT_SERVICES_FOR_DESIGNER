package domain

import (
	"database/sql/driver"
	"encoding/json"
	"errors"
	"fmt"
)

// JSONMap map vao cot jsonb cua Postgres.
// Dung cho cac field "mo rong" (metadata, payload) de khong phai migrate lien tuc.
type JSONMap map[string]any

func (m JSONMap) Value() (driver.Value, error) {
	if m == nil {
		return nil, nil
	}
	return json.Marshal(m)
}

func (m *JSONMap) Scan(src any) error {
	if src == nil {
		*m = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("domain: cannot scan %T into JSONMap", src)
	}
	if len(b) == 0 {
		*m = nil
		return nil
	}
	return json.Unmarshal(b, m)
}

// GormDataType bao cho GORM biet kieu cot khi AutoMigrate.
func (JSONMap) GormDataType() string { return "jsonb" }

// ParseJSONMap doc JSON string tu request (cho phep rong).
func ParseJSONMap(s string) (JSONMap, error) {
	if s == "" {
		return nil, nil
	}
	var m JSONMap
	if err := json.Unmarshal([]byte(s), &m); err != nil {
		return nil, errors.New("metadata/payload phai la JSON object hop le")
	}
	return m, nil
}

func (m JSONMap) String() string {
	if m == nil {
		return ""
	}
	b, err := json.Marshal(m)
	if err != nil {
		return ""
	}
	return string(b)
}

// JSONStrings map mang chuoi vao cot jsonb (vd sources, team_codes).
type JSONStrings []string

func (s JSONStrings) Value() (driver.Value, error) {
	if s == nil {
		return nil, nil
	}
	return json.Marshal(s)
}

func (s *JSONStrings) Scan(src any) error {
	if src == nil {
		*s = nil
		return nil
	}
	var b []byte
	switch v := src.(type) {
	case []byte:
		b = v
	case string:
		b = []byte(v)
	default:
		return fmt.Errorf("domain: cannot scan %T into JSONStrings", src)
	}
	if len(b) == 0 {
		*s = nil
		return nil
	}
	return json.Unmarshal(b, s)
}

func (JSONStrings) GormDataType() string { return "jsonb" }

// Has kiem tra co phan tu (dung cho sources/team_codes).
func (s JSONStrings) Has(v string) bool {
	for _, item := range s {
		if item == v {
			return true
		}
	}
	return false
}
