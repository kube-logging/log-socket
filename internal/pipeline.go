package internal

import (
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"strings"
	"unicode/utf8"

	"github.com/banzaicloud/log-socket/log"
	"github.com/wasmerio/wasmer-go/wasmer"
	"go.uber.org/multierr"
)

type Pipeline []*Stage

func (p Pipeline) ProcessRecord(record []byte) (res [][]byte, err error) {
	res = [][]byte{record}
	for _, stage := range p {
		inputs := res
		res = nil
		for _, input := range inputs {
			stage.data = input
			if err = stage.Receive(); err != nil {
				return
			}
			res = append(res, stage.sent...)
		}
	}
	return
}

func (p Pipeline) String() string {
	var b strings.Builder
	_, _ = b.WriteString("[")
	for i, s := range p {
		if i > 0 {
			_, _ = b.WriteString(" > ")
		}
		_, _ = fmt.Fprintf(&b, "%q", s.Origin)
	}
	_, _ = b.WriteString("]")
	return b.String()
}

type Stage struct {
	Instance *wasmer.Instance
	Memory   *wasmer.Memory
	Module   *wasmer.Module
	Origin   string

	data []byte
	err  error
	sent [][]byte
}

func (s *Stage) Receive() error {
	fn, err := s.Instance.Exports.GetFunction("receive")
	if err != nil {
		return err
	}
	if _, err := fn(len(s.data)); err != nil {
		return err
	}
	if err := s.err; err != nil {
		s.err = nil
		return err
	}
	return nil
}

func LoadStageFromFile(store *wasmer.Store, logs log.Sink, path string) (stage *Stage, err error) {
	stage = &Stage{
		Origin: path,
	}
	file, err := os.Open(path)
	if err != nil {
		return
	}
	code, err := io.ReadAll(file)
	if err != nil {
		return
	}
	stage.Module, err = wasmer.NewModule(store, code)
	if err != nil {
		return
	}
	imports := wasmer.NewImportObject()
	for _, desc := range stage.Module.Imports() {
		desc := desc
		if desc.Type().Kind() == wasmer.MEMORY {
			limits, err := wasmer.NewLimits(1, math.MaxUint32)
			if err != nil {
				panic(err)
			}
			stage.Memory = wasmer.NewMemory(store, wasmer.NewMemoryType(limits))
			imports.Register(desc.Module(), map[string]wasmer.IntoExtern{desc.Name(): stage.Memory})
			continue
		}
		if desc.Type().Kind() == wasmer.FUNCTION {
			switch desc.Name() {
			case "error":
				errorFnType := wasmer.NewFunctionType(wasmer.NewValueTypes(wasmer.I32, wasmer.I32), wasmer.NewValueTypes())
				imports.Register(desc.Module(), map[string]wasmer.IntoExtern{
					desc.Name(): wasmer.NewFunction(store, errorFnType, loggedFn(logs, desc, stage, func(v []wasmer.Value) ([]wasmer.Value, error) {
						a, l := v[0].I32(), v[1].I32()
						msg := stage.Memory.Data()[a : a+l]
						if !utf8.Valid(msg) {
							return nil, errors.New("message is not valid UTF-8")
						}
						err := errors.New(string(msg))
						stage.err = multierr.Append(stage.err, err)
						log.Event(logs, "stage produced an error", log.Fields{"stage": stage}, log.Error(err))
						return nil, nil
					})),
				})
			case "get_data":
				getDataFnTyp := wasmer.NewFunctionType(wasmer.NewValueTypes(wasmer.I32), wasmer.NewValueTypes())
				imports.Register(desc.Module(), map[string]wasmer.IntoExtern{
					desc.Name(): wasmer.NewFunction(store, getDataFnTyp, loggedFn(logs, desc, stage, func(v []wasmer.Value) ([]wasmer.Value, error) {
						a := v[0].I32()
						data := stage.data
						stage.data = nil
						if data == nil {
							return nil, errors.New("stage tried to get data when there was no data set")
						}
						if l := copy(stage.Memory.Data()[a:], data); l < len(data) {
							return nil, fmt.Errorf("could not copy whole data to memory at %x: %d < %d", a, l, len(data))
						}
						return nil, nil
					})),
				})
			case "send":
				sendFnTyp := wasmer.NewFunctionType(wasmer.NewValueTypes(wasmer.I32, wasmer.I32), wasmer.NewValueTypes(wasmer.I32))
				imports.Register(desc.Module(), map[string]wasmer.IntoExtern{
					desc.Name(): wasmer.NewFunction(store, sendFnTyp, loggedFn(logs, desc, stage, func(v []wasmer.Value) ([]wasmer.Value, error) {
						a, l := v[0].I32(), v[1].I32()
						data := make([]byte, l)
						_ = copy(data, stage.Memory.Data()[a:a+l])
						stage.sent = append(stage.sent, data)
						return []wasmer.Value{wasmer.NewI32(1)}, nil
					})),
				})
			/*
				case "fd_write":
					imports.Register(desc.Module(), map[string]wasmer.IntoExtern{
						desc.Name(): wasmer.NewFunction(store, desc.Type().IntoFunctionType(), func(v []wasmer.Value) ([]wasmer.Value, error) {
							fd, iovs_ptr, iovs_len, nwritten_ptr := v[0].I32(), v[1].I32(), v[2].I32(), v[3].I32()
							mem := stage.Memory.Data()
							ios := make([]string, iovs_len)
							nwritten := uint32(0)
							for i := range ios {
								iov_ptr := int(iovs_ptr) + i*8
								p, l := binary.LittleEndian.Uint32(mem[iov_ptr:]), binary.LittleEndian.Uint32(mem[iov_ptr+4:])
								blck := mem[p : p+l]
								data := make([]byte, l)
								copy(data, blck)
								ios[i] = string(data)
								nwritten += l
							}
							binary.LittleEndian.PutUint32(mem[nwritten_ptr:], nwritten)
							log.Event(logs, "fd_write", log.Fields{
								"stage":     stage,
								"fd":        fd,
								"&iovs":     iovs_ptr,
								"iovs_len":  iovs_len,
								"&nwritten": nwritten_ptr,
								"ios":       ios,
								"nwritten":  nwritten,
							})
							return []wasmer.Value{wasmer.NewI32(0)}, nil
						}),
					})
			*/
			default:
				imports.Register(desc.Module(), map[string]wasmer.IntoExtern{
					desc.Name(): wasmer.NewFunction(store, desc.Type().IntoFunctionType(), loggedFn(logs, desc, stage, func(v []wasmer.Value) ([]wasmer.Value, error) {
						return nil, nil
					})),
				})
				log.Event(logs, "stubbed import", log.V(1), log.Fields{"stage": stage, "import": fmt.Sprintf("(%q %q %s)", desc.Module(), desc.Name(), desc.Type().Kind())})
			}
			continue
		}
		return stage, fmt.Errorf("unsupported import (%q %q %s)", desc.Module(), desc.Name(), desc.Type().Kind())
	}
	stage.Instance, err = wasmer.NewInstance(stage.Module, imports)
	if err != nil {
		return
	}
	if stage.Memory == nil {
		stage.Memory, err = stage.Instance.Exports.GetMemory("memory")
		if err != nil {
			return
		}
	}
	return
}

func loggedFn(logs log.Sink, desc *wasmer.ImportType, stage *Stage, fn func([]wasmer.Value) ([]wasmer.Value, error)) func([]wasmer.Value) ([]wasmer.Value, error) {
	return func(args []wasmer.Value) ([]wasmer.Value, error) {
		log.Event(logs, "imported function invoked", log.V(2), log.Fields{
			"stage": stage,
			"func":  fmt.Sprintf("%s.%s", desc.Module(), desc.Name()),
			"args":  loggableArgs(args),
		})
		return fn(args)
	}
}

type loggableArgs []wasmer.Value

func (a loggableArgs) Format(f fmt.State, _ rune) {
	_, _ = fmt.Fprint(f, "[")
	for i, v := range a {
		if i > 0 {
			_, _ = fmt.Fprint(f, " ")
		}
		_, _ = fmt.Fprintf(f, "%+v", v.Unwrap())
	}
	_, _ = fmt.Fprint(f, "]")
}
