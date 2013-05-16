package goajax

import (
	"encoding/json"
	"errors"
	"log"
	"net/http"
	"reflect"
	"strconv"
	"strings"
	"sync"
	"unicode"
	"unicode/utf8"
)

type service struct {
	name   string                 // name of service
	rcvr   reflect.Value          // receiver of methods for the service
	typ    reflect.Type           // type of the receiver
	method map[string]*methodType // registered methods
}

type methodType struct {
	sync.Mutex // protects counters
	method     reflect.Method
	argTypes   []reflect.Type
	returnType reflect.Type
	numCalls   uint
}

type Server struct {
	sync.Mutex // protects the serviceMap
	serviceMap map[string]*service
}

type jsonRequest struct {
	Id     *json.RawMessage `json:"id"`
	Method string           `json:"method"`
	Params *json.RawMessage `json:"params"`
}

type jsonResponse struct {
	Id     *json.RawMessage `json:"id"`
	Result interface{}      `json:"result"`
	Error  interface{}      `json:"error"`
}

func NewServer() *Server {
	s := new(Server)
	s.serviceMap = make(map[string]*service)
	return s
}

// Precompute the reflect type for os.Error.  Can't use os.Error directly
// because Typeof takes an empty interface value.  This is annoying.
var unusedError *error
var typeOfOsError = reflect.TypeOf(unusedError).Elem()

func (server *Server) register(rcvr interface{}, name string, useName bool) error {
	server.Lock()
	defer server.Unlock()

	s := new(service)
	s.typ = reflect.TypeOf(rcvr)
	s.rcvr = reflect.ValueOf(rcvr)
	sname := reflect.Indirect(s.rcvr).Type().Name()
	if useName {
		sname = name
	}
	if sname == "" {
		log.Fatalln("rpc: no service name for type", s.typ.String())
	}
	if s.typ.PkgPath() != "" && !isExported(sname) && !useName {
		s := "rpc Register: type " + sname + " is not exported"
		log.Print(s)
		return errors.New(s)
	}
	if _, present := server.serviceMap[sname]; present {
		return errors.New("rpc: service already defined: " + sname)
	}
	s.name = sname
	s.method = make(map[string]*methodType)

	// Install the methods
	MethodLoop:
	for m := 0; m < s.typ.NumMethod(); m++ {
		method := s.typ.Method(m)
		mtype := method.Type
		mname := method.Name
		if mtype.PkgPath() != "" || !isExported(mname) {
			continue
		}

		args := []reflect.Type{}

		for i := 1; i < mtype.NumIn(); i++ {
			argType := mtype.In(i)
			if argPointerType := argType; argPointerType.Kind() == reflect.Ptr {
				if argPointerType.Elem().PkgPath() != "" && !isExported(argPointerType.Elem().Name()) {
					log.Println(mname, "argument type not exported:", argPointerType.Elem().Name())
					continue MethodLoop
				}
			}
			args = append(args, argType)
		}

		if mtype.NumOut() != 2 {
			log.Println("method", mname, "has wrong number of outs:", mtype.NumOut())
			continue
		}

		returnType := mtype.Out(0)
		if returnPointerType := returnType; returnPointerType.Kind() == reflect.Ptr {
			if returnPointerType.Elem().PkgPath() != "" && !isExported(returnPointerType.Elem().Name()) {
				log.Println(mname, "return type not exported:", returnPointerType.Elem().Name())
				continue
			}
		}

		if errorType := mtype.Out(1); errorType != typeOfOsError {
			log.Println("method", mname, "returns", errorType.String(), "not os.Error")
			continue
		}
		s.method[mname] = &methodType{method: method, argTypes: args, returnType: returnType}
	}

	if len(s.method) == 0 {
		s := "rpc Register: type " + sname + " has no exported methods of suitable type"
		log.Print(s)
		return errors.New(s)
	}
	server.serviceMap[s.name] = s
	return nil
}

func (server *Server) Register(rcvr interface{}) error {
	return server.register(rcvr, "", false)
}

func (server *Server) RegisterName(name string, rcvr interface{}) error {
	return server.register(rcvr, name, true)
}

func _new(t reflect.Type) reflect.Value {
	v := reflect.Zero(t)
	v.Set(reflect.Zero(t.Elem()).Addr())
	return v
}

// Is this an exported - upper case - name?
func isExported(name string) bool {
	rune, _ := utf8.DecodeRuneInString(name)
	return unicode.IsUpper(rune)
}

func (server *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {

	dec := json.NewDecoder(r.Body)
	req := new(jsonRequest)
	err := dec.Decode(req)

	if err != nil {
		s := "Invalid JSON-RPC."
		sendError(w, s)
		return
	}

	serviceMethod := strings.Split(req.Method, ".")
	server.Lock()
	service, ok := server.serviceMap[serviceMethod[0]]
	server.Unlock()

	if !ok {
		s := "Service not found."
		sendError(w, s)
		return
	}

	mtype, ok := service.method[serviceMethod[1]]
	if !ok {
		s := "Method not found."
		sendError(w, s)
		return
	}

	args, err := getParams(req, mtype.argTypes)

	if err != nil {
		sendError(w, err.Error())
		return
	}

	args = append([]reflect.Value{service.rcvr}, args...)

	mtype.Lock()
	mtype.numCalls++
	mtype.Unlock()
	function := mtype.method.Func

	returnValues := function.Call(args)

	// The return value for the method is an os.Error.
	errInter := returnValues[1].Interface()
	errmsg := ""
	if errInter != nil {
		errmsg = errInter.(error).Error()
	}

	resp := new(jsonResponse)

	if errmsg != "" {
		resp.Error = errmsg
	} else {
		resp.Result = returnValues[0].Interface()
	}

	resp.Id = req.Id

	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	enc := json.NewEncoder(w)
	enc.Encode(resp)
}

func sendError(w http.ResponseWriter, s string) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.Write([]byte("{\"jsonrpc\": \"2.0\", \"id\":null, \"error\":\"" + s + "\"}"))
}

func getParams(req *jsonRequest, argTypes []reflect.Type) ([]reflect.Value, error) {
	params := make([]*json.RawMessage, 0)
	err := json.Unmarshal(*req.Params, &params)

	if err != nil {
		return nil, err
	}

	if len(params) != len(argTypes) {
		return nil, errors.New("Incorrect number of parameters.")
	}

	args := make([]reflect.Value, 0, len(argTypes))

	for i, argType := range argTypes {
		argPointerType := argType

		if argPointerType.Kind() == reflect.Ptr {
			argPointer := reflect.New(argType)
			err := json.Unmarshal(*params[i], argPointer.Interface())
			if err != nil {
				return nil, errors.New("Type mismatch parameter " + strconv.Itoa(i+1) + ".")
			}
			args = append(args, reflect.Indirect(argPointer))
		} else {
			var arg reflect.Value
			var v interface{}
			err := json.Unmarshal(*params[i], &v)
			if err != nil {
				return nil, errors.New("Type mismatch parameter " + strconv.Itoa(i+1) + ".")
			}
			value := reflect.ValueOf(v)
			if value.Type() == argType {
				arg = reflect.ValueOf(v)
			} else if value.Type().Kind() == reflect.Float32 || value.Type().Kind() == reflect.Float64 {
				if argType.Kind() == reflect.Int || argType.Kind() == reflect.Int8 ||
					argType.Kind() == reflect.Int16 || argType.Kind() == reflect.Int32 ||
					argType.Kind() == reflect.Int64 {
					arg = reflect.ValueOf(int(v.(float64)))
				} else {
					return nil, errors.New("Type mismatch parameter " + strconv.Itoa(i+1) + ".")
				}
			} else {
				return nil, errors.New("Type mismatch parameter " + strconv.Itoa(i+1) + ".")
			}
			args = append(args, reflect.Value(arg))
		}
	}
	return args, nil
}
