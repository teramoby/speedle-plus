//Copyright (c) 2018, Oracle and/or its affiliates. All rights reserved.
//Licensed under the Universal Permissive License (UPL) Version 1.0 as shown at http://oss.oracle.com/licenses/upl.

package eval

import (
	"regexp"
	"sync"
)

// compiledExpressions caches each resource expression's compiled regexp, so
// a request no longer compiles every expression it tests.
var compiledExpressions sync.Map

func matchExpression(expression, s string) (bool, error) {
	if re, ok := compiledExpressions.Load(expression); ok {
		return re.(*regexp.Regexp).MatchString(s), nil
	}
	re, err := regexp.Compile(expression)
	if err != nil {
		return false, err
	}
	compiledExpressions.Store(expression, re)
	return re.MatchString(s), nil
}
