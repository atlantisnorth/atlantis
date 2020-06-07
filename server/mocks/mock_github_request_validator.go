// Code generated by pegomock. DO NOT EDIT.
// Source: github.com/runatlantis/atlantis/server (interfaces: RequestValidator)

package mocks

import (
	pegomock "github.com/petergtz/pegomock"
	http "net/http"
	"reflect"
	"time"
)

type MockRequestValidator struct {
	fail func(message string, callerSkip ...int)
}

func NewMockRequestValidator(options ...pegomock.Option) *MockRequestValidator {
	mock := &MockRequestValidator{}
	for _, option := range options {
		option.Apply(mock)
	}
	return mock
}

func (mock *MockRequestValidator) SetFailHandler(fh pegomock.FailHandler) { mock.fail = fh }
func (mock *MockRequestValidator) FailHandler() pegomock.FailHandler      { return mock.fail }

func (mock *MockRequestValidator) Validate(r *http.Request, secret []byte) ([]byte, error) {
	if mock == nil {
		panic("mock must not be nil. Use myMock := NewMockRequestValidator().")
	}
	params := []pegomock.Param{r, secret}
	result := pegomock.GetGenericMockFrom(mock).Invoke("Validate", params, []reflect.Type{reflect.TypeOf((*[]byte)(nil)).Elem(), reflect.TypeOf((*error)(nil)).Elem()})
	var ret0 []byte
	var ret1 error
	if len(result) != 0 {
		if result[0] != nil {
			ret0 = result[0].([]byte)
		}
		if result[1] != nil {
			ret1 = result[1].(error)
		}
	}
	return ret0, ret1
}

func (mock *MockRequestValidator) VerifyWasCalledOnce() *VerifierMockRequestValidator {
	return &VerifierMockRequestValidator{
		mock:                   mock,
		invocationCountMatcher: pegomock.Times(1),
	}
}

func (mock *MockRequestValidator) VerifyWasCalled(invocationCountMatcher pegomock.Matcher) *VerifierMockRequestValidator {
	return &VerifierMockRequestValidator{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
	}
}

func (mock *MockRequestValidator) VerifyWasCalledInOrder(invocationCountMatcher pegomock.Matcher, inOrderContext *pegomock.InOrderContext) *VerifierMockRequestValidator {
	return &VerifierMockRequestValidator{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
		inOrderContext:         inOrderContext,
	}
}

func (mock *MockRequestValidator) VerifyWasCalledEventually(invocationCountMatcher pegomock.Matcher, timeout time.Duration) *VerifierMockRequestValidator {
	return &VerifierMockRequestValidator{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
		timeout:                timeout,
	}
}

type VerifierMockRequestValidator struct {
	mock                   *MockRequestValidator
	invocationCountMatcher pegomock.Matcher
	inOrderContext         *pegomock.InOrderContext
	timeout                time.Duration
}

func (verifier *VerifierMockRequestValidator) Validate(r *http.Request, secret []byte) *MockRequestValidator_Validate_OngoingVerification {
	params := []pegomock.Param{r, secret}
	methodInvocations := pegomock.GetGenericMockFrom(verifier.mock).Verify(verifier.inOrderContext, verifier.invocationCountMatcher, "Validate", params, verifier.timeout)
	return &MockRequestValidator_Validate_OngoingVerification{mock: verifier.mock, methodInvocations: methodInvocations}
}

type MockRequestValidator_Validate_OngoingVerification struct {
	mock              *MockRequestValidator
	methodInvocations []pegomock.MethodInvocation
}

func (c *MockRequestValidator_Validate_OngoingVerification) GetCapturedArguments() (*http.Request, []byte) {
	r, secret := c.GetAllCapturedArguments()
	return r[len(r)-1], secret[len(secret)-1]
}

func (c *MockRequestValidator_Validate_OngoingVerification) GetAllCapturedArguments() (_param0 []*http.Request, _param1 [][]byte) {
	params := pegomock.GetGenericMockFrom(c.mock).GetInvocationParams(c.methodInvocations)
	if len(params) > 0 {
		_param0 = make([]*http.Request, len(c.methodInvocations))
		for u, param := range params[0] {
			_param0[u] = param.(*http.Request)
		}
		_param1 = make([][]byte, len(c.methodInvocations))
		for u, param := range params[1] {
			_param1[u] = param.([]byte)
		}
	}
	return
}
