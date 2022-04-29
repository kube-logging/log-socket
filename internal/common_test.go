package internal

import (
	"reflect"
	"testing"
)

func TestGetIn(t *testing.T) {
	type args struct {
		m    interface{}
		args []interface{}
	}
	tests := []struct {
		name string
		args args
		want interface{}
	}{
		{
			name: "nil map, nil args",
			args: args{m: nil, args: nil},
			want: nil,
		},
		{
			name: "nil map, empty args",
			args: args{m: nil, args: []interface{}{}},
			want: nil,
		},
		{
			name: "nil map, args with key",
			args: args{m: nil, args: []interface{}{"should not exists"}},
			want: nil,
		},
		{
			name: "empty map, nil args",
			args: args{m: Strimap{}, args: nil},
			want: Strimap{},
		},
		{
			name: "empty map, empty args",
			args: args{m: Strimap{}, args: []interface{}{}},
			want: Strimap{},
		},
		{
			name: "empty map, args with key",
			args: args{m: Strimap{}, args: []interface{}{"should not exists"}},
			want: nil,
		},
		{
			name: "non existing keys",
			args: args{m: Strimap{"foo": "bar"}, args: []interface{}{"baz"}},
			want: nil,
		},
		{
			name: "invalid key type - map / int",
			args: args{m: Strimap{"foo": "bar"}, args: []interface{}{3}},
			want: nil,
		},
		{
			name: "invalid key type - array / string",
			args: args{m: []interface{}{"foo", "bar"}, args: []interface{}{"foo"}},
			want: nil,
		},
		{
			name: "array - index out of bounds / lower",
			args: args{m: []interface{}{"foo", "bar"}, args: []interface{}{-1}},
			want: nil,
		},
		{
			name: "array - index out of bounds / higher",
			args: args{m: []interface{}{"foo", "bar"}, args: []interface{}{3}},
			want: nil,
		},
		{
			name: "nested map",
			args: args{m: Strimap{"kubernetes": Strimap{"labels": Strimap{"mylabel": "myvalue"}}}, args: []interface{}{"kubernetes", "labels", "mylabel"}},
			want: "myvalue",
		},
		{
			name: "nested array",
			args: args{m: []interface{}{[]interface{}{"foo", "bar"}, []interface{}{"baz", "err"}}, args: []interface{}{1, 1}},
			want: "err",
		},
		{
			name: "mixed map / array",
			args: args{m: Strimap{"kubernetes": Strimap{"labels": Strimap{"mylabel": []interface{}{"myvalue1", "myvalue2"}}}}, args: []interface{}{"kubernetes", "labels", "mylabel", 1}},
			want: "myvalue2",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := GetIn(tt.args.m, tt.args.args...); !reflect.DeepEqual(got, tt.want) {
				t.Errorf("GetIn() = %v, want %v", got, tt.want)
			}
		})
	}
}
