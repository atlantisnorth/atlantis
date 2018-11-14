package matchers

import (
	"reflect"

	"github.com/petergtz/pegomock"
	events "github.com/runatlantis/atlantis/server/events"
)

func AnyPtrToEventsTryLockResponse() *events.TryLockResponse {
	pegomock.RegisterMatcher(pegomock.NewAnyMatcher(reflect.TypeOf((*(*events.TryLockResponse))(nil)).Elem()))
	var nullValue *events.TryLockResponse
	return nullValue
}

func EqPtrToEventsTryLockResponse(value *events.TryLockResponse) *events.TryLockResponse {
	pegomock.RegisterMatcher(&pegomock.EqMatcher{Value: value})
	var nullValue *events.TryLockResponse
	return nullValue
}
