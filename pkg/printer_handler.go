package pkg

import (
	"encoding/json"
	"fmt"
	"reflect"
)

func Marker(color IoColor, text any, other_text ...any) {
	fmt.Print(color, text)
	for _, ot := range other_text {
		fmt.Print(" ", ot)
	}
	fmt.Println(IoColorReset)
}

func Returns(text any, other_text ...any) {
	fmt.Print(IoColorYellow, "[", reflect.TypeOf(text), "] Return:", text)
	for _, ot := range other_text {
		fmt.Print(" ", ot)
	}
	fmt.Println(IoColorReset)
}

func StructPrettyPrint(data any, col IoColor) {
	jsonData, err := json.MarshalIndent(data, "", "  ")
	if err != nil {
		Marker(col, err.Error())
		Marker(col, data)
	}
	Marker(col, string(jsonData))
}

func UnitTestCheck(test_num int, test_name string, data any, expectedType reflect.Type) {
	dataType := reflect.TypeOf(data)
	Marker(IoColorYellow, data)
	if dataType != expectedType {
		Marker(IoColorRed, fmt.Sprintf("%d. %s [𐄂 fail]", test_num, test_name))
		return
	}
	Marker(IoColorGreen, fmt.Sprintf("%d. %s [✓ passed]", test_num, test_name))
}
