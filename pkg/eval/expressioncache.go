// Licensed under the Universal Permissive License (UPL) Version 1.0.

package eval

import "regexp"

// compiledExpression is a resource expression compiled once, with the number
// of references to it from the policies in a cache.
type compiledExpression struct {
	re   *regexp.Regexp
	err  error
	refs int
}

// ExpressionCache holds the compiled resource expressions of the policies in
// one policy cache. An expression is compiled when the first policy using it
// is added and dropped when the last one is deleted, so evaluation never
// compiles an expression and the cache never outgrows the policies.
//
// Like the rest of the policy cache, it is changed only under the service's
// write lock and read under its read lock.
type ExpressionCache struct {
	expressions map[string]*compiledExpression
}

// acquire adds a reference to expression, compiling it on first use.
func (c *ExpressionCache) acquire(expression string) {
	if entry, ok := c.expressions[expression]; ok {
		entry.refs++
		return
	}
	if c.expressions == nil {
		c.expressions = make(map[string]*compiledExpression)
	}
	re, err := regexp.Compile(expression)
	c.expressions[expression] = &compiledExpression{re: re, err: err, refs: 1}
}

// release drops a reference to expression, forgetting it after the last one.
func (c *ExpressionCache) release(expression string) {
	entry, ok := c.expressions[expression]
	if !ok {
		return
	}
	entry.refs--
	if entry.refs <= 0 {
		delete(c.expressions, expression)
	}
}

// matchString reports whether s contains a match of expression, as
// regexp.MatchString does. An expression that is not in the cache is compiled
// for this call only; the cache is not changed, since callers hold only the
// read lock.
func (c *ExpressionCache) matchString(expression, s string) (bool, error) {
	if c != nil {
		if entry, ok := c.expressions[expression]; ok {
			if entry.err != nil {
				return false, entry.err
			}
			return entry.re.MatchString(s), nil
		}
	}
	return regexp.MatchString(expression, s)
}

// size is the number of distinct expressions held.
func (c *ExpressionCache) size() int {
	return len(c.expressions)
}
