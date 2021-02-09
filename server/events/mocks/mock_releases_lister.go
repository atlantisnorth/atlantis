// Code generated by pegomock. DO NOT EDIT.
// Source: github.com/runatlantis/atlantis/server/events (interfaces: ReleasesLister)

package mocks

import (
	pegomock "github.com/petergtz/pegomock"
	"reflect"
	"time"
)

type MockReleasesLister struct {
	fail func(message string, callerSkip ...int)
}

func NewMockReleasesLister(options ...pegomock.Option) *MockReleasesLister {
	mock := &MockReleasesLister{}
	for _, option := range options {
		option.Apply(mock)
	}
	return mock
}

func (mock *MockReleasesLister) SetFailHandler(fh pegomock.FailHandler) { mock.fail = fh }
func (mock *MockReleasesLister) FailHandler() pegomock.FailHandler      { return mock.fail }

func (mock *MockReleasesLister) ListReleases() ([]string, error) {
	if mock == nil {
		panic("mock must not be nil. Use myMock := NewMockReleasesLister().")
	}
	params := []pegomock.Param{}
	result := pegomock.GetGenericMockFrom(mock).Invoke("ListReleases", params, []reflect.Type{reflect.TypeOf((*[]string)(nil)).Elem(), reflect.TypeOf((*error)(nil)).Elem()})
	var ret0 []string
	var ret1 error
	if len(result) != 0 {
		if result[0] != nil {
			ret0 = result[0].([]string)
		}
		if result[1] != nil {
			ret1 = result[1].(error)
		}
	}
	return ret0, ret1
}

func (mock *MockReleasesLister) VerifyWasCalledOnce() *VerifierMockReleasesLister {
	return &VerifierMockReleasesLister{
		mock:                   mock,
		invocationCountMatcher: pegomock.Times(1),
	}
}

func (mock *MockReleasesLister) VerifyWasCalled(invocationCountMatcher pegomock.Matcher) *VerifierMockReleasesLister {
	return &VerifierMockReleasesLister{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
	}
}

func (mock *MockReleasesLister) VerifyWasCalledInOrder(invocationCountMatcher pegomock.Matcher, inOrderContext *pegomock.InOrderContext) *VerifierMockReleasesLister {
	return &VerifierMockReleasesLister{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
		inOrderContext:         inOrderContext,
	}
}

func (mock *MockReleasesLister) VerifyWasCalledEventually(invocationCountMatcher pegomock.Matcher, timeout time.Duration) *VerifierMockReleasesLister {
	return &VerifierMockReleasesLister{
		mock:                   mock,
		invocationCountMatcher: invocationCountMatcher,
		timeout:                timeout,
	}
}

type VerifierMockReleasesLister struct {
	mock                   *MockReleasesLister
	invocationCountMatcher pegomock.Matcher
	inOrderContext         *pegomock.InOrderContext
	timeout                time.Duration
}

func (verifier *VerifierMockReleasesLister) ListReleases() *MockReleasesLister_ListReleases_OngoingVerification {
	params := []pegomock.Param{}
	methodInvocations := pegomock.GetGenericMockFrom(verifier.mock).Verify(verifier.inOrderContext, verifier.invocationCountMatcher, "ListReleases", params, verifier.timeout)
	return &MockReleasesLister_ListReleases_OngoingVerification{mock: verifier.mock, methodInvocations: methodInvocations}
}

type MockReleasesLister_ListReleases_OngoingVerification struct {
	mock              *MockReleasesLister
	methodInvocations []pegomock.MethodInvocation
}

func (c *MockReleasesLister_ListReleases_OngoingVerification) GetCapturedArguments() {
}

func (c *MockReleasesLister_ListReleases_OngoingVerification) GetAllCapturedArguments() {
}
