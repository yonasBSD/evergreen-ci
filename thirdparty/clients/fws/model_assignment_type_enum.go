/*
Foliage Web Services

Foliage web services, owner: DevProd Services & Integrations team

API version: 1.0.0
*/

// Code generated by OpenAPI Generator (https://openapi-generator.tech); DO NOT EDIT.

package fws

import (
	"encoding/json"
	"fmt"
)

// AssignmentTypeEnum Enum for assignment types.
type AssignmentTypeEnum string

// List of AssignmentTypeEnum
const (
	OFFENDING_VERSION_ID     AssignmentTypeEnum = "offending version id"
	FAILURE_METADATA_TAG     AssignmentTypeEnum = "failure metadata tag"
	SYSTEM_AND_SETUP_FAILURE AssignmentTypeEnum = "system and setup failure"
	TEST_FILE_NAME           AssignmentTypeEnum = "test file name"
	TASK_TAG                 AssignmentTypeEnum = "task tag"
	BUILD_VARIANT_TAG        AssignmentTypeEnum = "build variant tag"
	TASK_TO_TEAM_MAPPING     AssignmentTypeEnum = "task to team mapping"
	DEFAULT_TEAM             AssignmentTypeEnum = "default team"
)

// All allowed values of AssignmentTypeEnum enum
var AllowedAssignmentTypeEnumEnumValues = []AssignmentTypeEnum{
	"offending version id",
	"failure metadata tag",
	"system and setup failure",
	"test file name",
	"task tag",
	"build variant tag",
	"task to team mapping",
	"default team",
}

func (v *AssignmentTypeEnum) UnmarshalJSON(src []byte) error {
	var value string
	err := json.Unmarshal(src, &value)
	if err != nil {
		return err
	}
	enumTypeValue := AssignmentTypeEnum(value)
	for _, existing := range AllowedAssignmentTypeEnumEnumValues {
		if existing == enumTypeValue {
			*v = enumTypeValue
			return nil
		}
	}

	return fmt.Errorf("%+v is not a valid AssignmentTypeEnum", value)
}

// NewAssignmentTypeEnumFromValue returns a pointer to a valid AssignmentTypeEnum
// for the value passed as argument, or an error if the value passed is not allowed by the enum
func NewAssignmentTypeEnumFromValue(v string) (*AssignmentTypeEnum, error) {
	ev := AssignmentTypeEnum(v)
	if ev.IsValid() {
		return &ev, nil
	} else {
		return nil, fmt.Errorf("invalid value '%v' for AssignmentTypeEnum: valid values are %v", v, AllowedAssignmentTypeEnumEnumValues)
	}
}

// IsValid return true if the value is valid for the enum, false otherwise
func (v AssignmentTypeEnum) IsValid() bool {
	for _, existing := range AllowedAssignmentTypeEnumEnumValues {
		if existing == v {
			return true
		}
	}
	return false
}

// Ptr returns reference to AssignmentTypeEnum value
func (v AssignmentTypeEnum) Ptr() *AssignmentTypeEnum {
	return &v
}

type NullableAssignmentTypeEnum struct {
	value *AssignmentTypeEnum
	isSet bool
}

func (v NullableAssignmentTypeEnum) Get() *AssignmentTypeEnum {
	return v.value
}

func (v *NullableAssignmentTypeEnum) Set(val *AssignmentTypeEnum) {
	v.value = val
	v.isSet = true
}

func (v NullableAssignmentTypeEnum) IsSet() bool {
	return v.isSet
}

func (v *NullableAssignmentTypeEnum) Unset() {
	v.value = nil
	v.isSet = false
}

func NewNullableAssignmentTypeEnum(val *AssignmentTypeEnum) *NullableAssignmentTypeEnum {
	return &NullableAssignmentTypeEnum{value: val, isSet: true}
}

func (v NullableAssignmentTypeEnum) MarshalJSON() ([]byte, error) {
	return json.Marshal(v.value)
}

func (v *NullableAssignmentTypeEnum) UnmarshalJSON(src []byte) error {
	v.isSet = true
	return json.Unmarshal(src, &v.value)
}
