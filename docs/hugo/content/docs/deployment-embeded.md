+++
title = "Speedle embedded mode"
description = ""
weight = 12
draft = false
toc = true
tocheading = "h2"
tocsidebar = false
bref = ""
+++

## What is embedded mode?

Unlike service mode, embedded mode runs the Speedle policy decision engine inside
the calling process. Applications call the evaluator API directly to obtain
authorization decisions.

## Limitations

- Only Go applications are supported. The original setup instructions use Go 1.10.
- Policies must use the file store.

## Using embedded mode

### Add Speedle to your project

Download the Speedle source using one of the following approaches.

1. Use `go get`:

```bash
go get github.com/teramoby/speedle-plus
```

2. Use `dep`. Add this constraint to `Gopkg.toml`:

```toml
[[constraint]]
  name = "github.com/teramoby/speedle-plus"
  branch = "master"
```

Then add the source to your `vendor` directory:

```bash
dep ensure -update github.com/teramoby/speedle-plus
```

### Initialize an evaluator

`func eval.NewFromFile(loc string, isWatch bool) (ads.PolicyEvaluator, error)`
initializes an evaluator from a policy file. It takes two arguments:

- `loc`: the policy file location. JSON and SPDL formats are supported.
- `isWatch`: when `true`, the evaluator reloads policies when the file changes;
  when `false`, it does not watch for changes.

The method returns an evaluator instance. For example:

```go
import (
  "github.com/teramoby/speedle-plus/pkg/eval"
)

func foo() {
    evaluator, err := eval.NewFromFile(spdlLoc, true)
    // Handle err before using evaluator.
    _, _ = evaluator, err
}
```

### Request an authorization decision

`IsAllowed` evaluates a request against the policies. The `ads.RequestContext`
argument contains the subject, service name, resource, action and attributes:

```go
type RequestContext struct {
  Subject     *ads.Subject
  ServiceName string
  Resource    string
  Action      string
  Attributes  map[string]interface{}
}
```

The result has three values:

1. `allowed bool`: whether the subject may perform the action on the resource.
2. `reason Reason`: the reason for the decision.
3. `err error`: an evaluation error, or `nil` if evaluation succeeds.

## Example

See the [embedded expenses example](https://github.com/teramoby/speedle-plus/tree/master/samples/embedded/expenses).
