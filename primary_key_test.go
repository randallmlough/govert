package govert

import (
	"reflect"
	"testing"
)

func Test_primaryKeyType(t *testing.T) {

	type ID int
	var customID ID

	type CustomStruct struct {
		ID string
	}
	type args struct {
		typ interface{}
	}
	tests := []struct {
		name string
		args args
		want reflect.Type
	}{
		{
			name: "uint",
			args: args{
				int(1),
			},
			want: reflect.TypeOf(int(-1)),
		},
		{
			name: "uint",
			args: args{
				uint(1),
			},
			want: reflect.TypeOf(uint(3)),
		},
		{
			name: "string",
			args: args{
				string("string"),
			},
			want: reflect.TypeOf(string("value shouldn't matter")),
		},
		{
			name: "custom ID",
			args: args{
				customID,
			},
			want: reflect.TypeOf(ID(1)),
		},
		{
			name: "custom struct",
			args: args{
				CustomStruct{ID: ""},
			},
			want: reflect.TypeOf(CustomStruct{ID: "value shouldn't matter"}),
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := primaryKeyType(tt.args.typ); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("primaryKeyType() = %v, want %v", got, tt.want)
			}
		})
	}
}
