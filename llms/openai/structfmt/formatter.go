package structfmt

import (
	"reflect"
	"strings"

	"github.com/tmc/langchaingo/llms/openai"
)

func StructToOAIRespFormat(input any) *openai.ResponseFormat {
	var respFormat openai.ResponseFormat
	schema := openai.ResponseFormatJSONSchema{}
	respFormat.Type = "json_schema"
	respFormat.JSONSchema = &schema
	schema.Name = "object"
	schema.Strict = true
	schema.Schema = structToOAIRespFormat(input)
	return &respFormat
}

func structToOAIRespFormat(input any) *openai.ResponseFormatJSONSchemaProperty {
	prop := openai.ResponseFormatJSONSchemaProperty{}
	if reflect.TypeOf(input).Kind() == reflect.Ptr {
		if reflect.ValueOf(input).IsNil() {
			return structToOAIRespFormat(reflect.New(reflect.TypeOf(input).Elem()).Elem().Interface())
		}
		return structToOAIRespFormat(reflect.ValueOf(input).Elem().Interface())
	}
	if reflect.TypeOf(input).Kind() == reflect.Struct {
		prop.Type = "object"
		prop.Properties = make(map[string]*openai.ResponseFormatJSONSchemaProperty)
		prop.Required = []string{}
		for i := range reflect.TypeOf(input).NumField() {
			field := reflect.TypeOf(input).Field(i)
			if !field.IsExported() {
				continue
			}
			fieldName, ok := field.Tag.Lookup("json")
			if !ok {
				continue
			}
			fieldName = strings.Split(fieldName, ",")[0]
			if fieldName == "-" {
				continue
			}
			description, ok := field.Tag.Lookup("description")
			if !ok {
				panic("No description found for field: " + field.Name)
			}
			if description == "-" {
				continue
			}
			prop.Properties[fieldName] = structToOAIRespFormat(reflect.ValueOf(input).Field(i).Interface())
			prop.Properties[fieldName].Description = description
			singular, ok := field.Tag.Lookup("singular")
			if ok {
				if field.Type.Kind() != reflect.Slice {
					panic("Singular tag found on non-slice field: " + field.Name)
				}
				if field.Type.Elem().Kind() == reflect.Struct {
					panic("Singular tag found on struct slice field: " + field.Name)
				}
				prop.Properties[fieldName].Items.Description = singular
			} else {
				if field.Type.Kind() == reflect.Slice && field.Type.Elem().Kind() != reflect.Struct {
					panic("No singular tag found on slice field: " + field.Name)
				}
			}
			prop.Required = append(prop.Required, fieldName)
		}
	} else if reflect.TypeOf(input).Kind() == reflect.Slice {
		prop.Type = "array"
		itemInstance := reflect.New(reflect.TypeOf(input).Elem())
		prop.Items = structToOAIRespFormat(itemInstance.Interface())
	} else {
		prop.Type = reflect.TypeOf(input).Kind().String()
		if prop.Type == "bool" {
			prop.Type = "boolean"
		}
	}
	return &prop
}
