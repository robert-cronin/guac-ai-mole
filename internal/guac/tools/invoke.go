package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"reflect"

	"github.com/Khan/genqlient/graphql"
)

// InvokeGUACOperation looks up an operation by its function name, unmarshals the given JSON
// into the filter parameter, calls the operation, and returns the result.
//
// fnName is the full function name as known by runtime.FuncForPC (e.g. "github.com/guacsec/guac/pkg/assembler/clients/generated.Dependencies").
// data is the raw JSON from the LLM which should match the filter schema.
// It returns the response interface and any error encountered.
func InvokeGUACOperation(ctx context.Context, guacClient graphql.Client, fnName string, data []byte) (interface{}, error) {
	// Find the operation in allowedOperations
	opEntry, err := findOperationByName(fnName)
	if err != nil {
		return nil, err
	}

	// Unmarshal the filter
	filterVal, err := unmarshalFilterFromJSON(opEntry.Operation, data)
	if err != nil {
		return nil, err
	}

	// Call the operation
	return callOperation(ctx, guacClient, opEntry.Operation, filterVal)
}

// findOperationByName searches allowedOperations for an operation whose runtime name matches fnName.
func findOperationByName(fnName string) (*allowedOperationType, error) {
	for _, ao := range allowedOperations {
		aoName := GetRuntimeFuncName(ao.Operation)
		if aoName == fnName {
			return &ao, nil
		}
	}

	return nil, fmt.Errorf("operation with name %q not found", fnName)
}

// unmarshalFilterFromJSON takes the operation function and data, then determines the filter type and unmarshals data into it.
func unmarshalFilterFromJSON(fn interface{}, data []byte) (interface{}, error) {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnVal.Kind() != reflect.Func {
		return nil, fmt.Errorf("provided operation is not a function")
	}

	numIn := fnType.NumIn()
	if numIn < 3 {
		return nil, fmt.Errorf("operation does not have the expected number of arguments")
	}

	// The last parameter is assumed to be the filter type
	filterType := fnType.In(numIn - 1)

	// Create a new instance of the filter type
	isPtr := filterType.Kind() == reflect.Ptr
	var filterVal reflect.Value
	if isPtr {
		filterVal = reflect.New(filterType.Elem())
	} else {
		filterVal = reflect.New(filterType)
	}

	// Unmarshal JSON into the filter instance
	if err := json.Unmarshal(data, filterVal.Interface()); err != nil {
		return nil, fmt.Errorf("failed to unmarshal JSON into filter type %s: %w", filterType.String(), err)
	}

	// If the filter was initially not a pointer, return its elem
	if !isPtr {
		return filterVal.Elem().Interface(), nil
	}
	return filterVal.Interface(), nil
}

// callOperation calls the operation function with the given ctx, guacClient, and filter arguments.
// The operation is expected to have the signature: func(ctx context.Context, client graphql.Client, filter T) (*R, error)
func callOperation(ctx context.Context, guacClient graphql.Client, fn interface{}, filter interface{}) (interface{}, error) {
	fnVal := reflect.ValueOf(fn)
	fnType := fnVal.Type()

	if fnVal.Kind() != reflect.Func {
		return nil, fmt.Errorf("provided operation is not a function")
	}

	numIn := fnType.NumIn()
	if numIn < 3 {
		return nil, fmt.Errorf("operation does not have the expected number of arguments")
	}

	// Prepare arguments
	args := []reflect.Value{
		reflect.ValueOf(ctx),
		reflect.ValueOf(guacClient),
		reflect.ValueOf(filter),
	}

	// Call the function
	resVals := fnVal.Call(args)
	if len(resVals) != 2 {
		return nil, fmt.Errorf("operation does not return the expected number of values (expected 2)")
	}

	resVal := resVals[0] // *ResponseType
	errVal := resVals[1] // error

	if !errVal.IsNil() {
		errInterface := errVal.Interface().(error)
		slog.Error("operation call returned error", "error", errInterface)
		return nil, errInterface
	}

	return resVal.Interface(), nil
}
