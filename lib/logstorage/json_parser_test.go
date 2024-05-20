package logstorage

import (
	"reflect"
	"testing"
)

func TestJSONParserFailure(t *testing.T) {
	f := func(data string) {
		t.Helper()

		p := GetJSONParser()
		err := p.ParseLogMessage([]byte(data), "")
		if err == nil {
			t.Fatalf("expecting non-nil error")
		}
		PutJSONParser(p)
	}
	f("")
	f("{foo")
	f("[1,2,3]")
	f(`{"foo",}`)
}

func TestJSONParserSuccess(t *testing.T) {
	f := func(data, prefix string, fieldsExpected []Field) {
		t.Helper()

		p := GetJSONParser()
		err := p.ParseLogMessage([]byte(data), prefix)
		if err != nil {
			t.Fatalf("unexpected error: %s", err)
		}
		if !reflect.DeepEqual(p.Fields, fieldsExpected) {
			t.Fatalf("unexpected fields;\ngot\n%s\nwant\n%s", p.Fields, fieldsExpected)
		}
		PutJSONParser(p)
	}

	f("{}", "", nil)
	f(`{"foo":"bar"}`, "", []Field{
		{
			Name:  "foo",
			Value: "bar",
		},
	})
	f(`{"foo":"bar"}`, "prefix_", []Field{
		{
			Name:  "prefix_foo",
			Value: "bar",
		},
	})
	f(`{"foo":{"bar":"baz"},"a":1,"b":true,"c":[1,2],"d":false}`, "", []Field{
		{
			Name:  "foo.bar",
			Value: "baz",
		},
		{
			Name:  "a",
			Value: "1",
		},
		{
			Name:  "b",
			Value: "true",
		},
		{
			Name:  "c",
			Value: "[1,2]",
		},
		{
			Name:  "d",
			Value: "false",
		},
	})
	f(`{"foo":{"bar":"baz"},"a":1,"b":true,"c":[1,2],"d":false}`, "prefix_", []Field{
		{
			Name:  "prefix_foo.bar",
			Value: "baz",
		},
		{
			Name:  "prefix_a",
			Value: "1",
		},
		{
			Name:  "prefix_b",
			Value: "true",
		},
		{
			Name:  "prefix_c",
			Value: "[1,2]",
		},
		{
			Name:  "prefix_d",
			Value: "false",
		},
	})
}
