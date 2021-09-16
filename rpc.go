// package rpc is a proof of concept RPC based on Go's net/rpc with a couple of tweaks.
// - adds support for JSON payloads
// - adds support for streaming using SSE
package rpc

import (
	"bytes"
	"encoding/json"
	"fmt"
	"go/token"
	"net/http"
	"reflect"
)

type (
	Service struct {
		Methods map[string]Method
	}
	Method struct {
		Name         string
		Receiver     reflect.Value
		Method       reflect.Method
		RequestType  reflect.Type
		ResponseType reflect.Type
	}
	Request struct {
		ServiceMethod string          // format: "Service.Method"
		Body          json.RawMessage // body of request
		Seq           uint64          // sequence number chosen by client
	}
	Response struct {
		ServiceMethod string          // echoes that of the Request
		Body          json.RawMessage // body of request
		Seq           uint64          // echoes that of the request
		Error         string          // error, if any.
	}
)

func New() *Service {
	return &Service{
		Methods: map[string]Method{},
	}
}

var typeOfError = reflect.TypeOf((*error)(nil)).Elem()

// Register the given interface and register all the methods.
// This method is not thread safe.
func (s *Service) Register(i interface{}) error {
	it := reflect.TypeOf(i)
	iv := reflect.ValueOf(i)

	name := reflect.Indirect(iv).Type().Name()
	if name == "" {
		return fmt.Errorf("rpc: type name not found")
	}

	if !token.IsExported(name) {
		return fmt.Errorf("rpc: type name %s is not exported", name)
	}

	for m := 0; m < it.NumMethod(); m++ {
		method := it.Method(m)
		methodType := method.Type
		methodName := name + "." + method.Name

		if method.PkgPath != "" {
			continue
		}

		if methodType.NumIn() != 3 {
			continue
		}

		requestType := methodType.In(1)
		if requestType.Kind() != reflect.Ptr {
			continue
		}

		if !token.IsExported(requestType.Name()) && requestType.PkgPath() != "" {
			continue
		}

		responseType := methodType.In(2)
		if responseType.Kind() != reflect.Ptr {
			continue
		}

		if !token.IsExported(responseType.Name()) && responseType.PkgPath() != "" {
			continue
		}

		if methodType.NumOut() != 1 {
			continue
		}

		returnType := methodType.Out(0)
		if returnType != typeOfError {
			continue
		}

		s.Methods[methodName] = Method{
			Name:         methodName,
			Receiver:     iv,
			Method:       method,
			RequestType:  requestType,
			ResponseType: responseType,
		}
	}

	return nil
}

func (s *Service) Call(httpClient *http.Client, uri string, method string, reqBody, resBody interface{}) error {
	// Look up method, fail if not found.
	m, ok := s.Methods[method]
	if !ok {
		return fmt.Errorf("rpc: can't find method %q", method)
	}

	// Encode the request.
	reqBodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return fmt.Errorf("rpc: error encoding request: %v", err)
	}

	req := Request{
		ServiceMethod: m.Name,
		Body:          reqBodyBytes,
	}
	reqBytes, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("rpc: error marshalling request: %v", err)
	}

	// Send the request.
	resp, err := httpClient.Post(uri, "application/json", bytes.NewReader(reqBytes))
	if err != nil {
		return fmt.Errorf("rpc: error sending request: %v", err)
	}

	// Decode the response.
	res := Response{}
	err = json.NewDecoder(resp.Body).Decode(&res)
	if err != nil {
		return fmt.Errorf("rpc: error reading response body: %v", err)
	}
	defer resp.Body.Close()

	// Handle error.
	if res.Error != "" {
		return fmt.Errorf("rpc: server: %s", res.Error)
	}

	err = json.Unmarshal(res.Body, resBody)
	if err != nil {
		return fmt.Errorf("rpc: %s", err)
	}

	return nil
}

func (s *Service) Serve() http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" {
			http.Error(w, "Method not allowed", http.StatusMethodNotAllowed)
			return
		}

		if r.Header.Get("Content-Type") != "application/json" {
			http.Error(w, "Content-Type must be application/json", http.StatusUnsupportedMediaType)
			return
		}

		// Decode the request.
		var req Request
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Look up method, fail if not found.
		m, ok := s.Methods[req.ServiceMethod]
		if !ok {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Decode the request body.
		reqBody := reflect.New(m.RequestType).Interface()
		err := json.Unmarshal(req.Body, reqBody)
		if err != nil {
			http.Error(w, "Bad request", http.StatusBadRequest)
			return
		}

		// Call the method, marshal the result.
		resBody := reflect.New(m.ResponseType.Elem())
		args := []reflect.Value{
			m.Receiver,
			reflect.ValueOf(reqBody).Elem(),
			resBody,
		}
		callRes := m.Method.Func.Call(args)
		if len(callRes) == 1 && callRes[0].Interface() != nil {
			err := callRes[0].Interface().(error)
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Encode response body
		resBodyBytes, err := json.Marshal(resBody.Interface())
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}

		// Write the response.
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		res := Response{
			ServiceMethod: req.ServiceMethod,
			Body:          resBodyBytes,
			Seq:           req.Seq,
		}
		err = json.NewEncoder(w).Encode(res)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
	})
}

// Is this type exported or a builtin?
func isExportedOrBuiltinType(t reflect.Type) bool {
	for t.Kind() == reflect.Ptr {
		t = t.Elem()
	}
	// PkgPath will be non-empty even for an exported type,
	// so we need to check the type name as well.
	return token.IsExported(t.Name()) || t.PkgPath() == ""
}
