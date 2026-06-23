<p align="center">
    <img src="/docs/images/sp_logo.png" height="50%" width="50%" class="center"/>
</p>
<p align="center">
    <a href="https://join.slack.com/t/speedleproject/shared_invite/enQtNTUzODM3NDY0ODE2LTg0ODc0NzQ1MjVmM2NiODVmMThkMmVjNmMyODA0ZWJjZjQ3NDc2MjdlMzliN2U4MDRkZjhlYzYzMDEyZTgxMGQ">
        <img src="https://img.shields.io/badge/slack-speedle-red.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/tags">
        <img src="https://img.shields.io/github/v/tag/teramoby/speedle-plus">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/issues">
        <img src="https://img.shields.io/github/issues/teramoby/speedle-plus.svg">
    </a>
    <a href="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml">
        <img src="https://github.com/teramoby/speedle-plus/actions/workflows/ci.yml/badge.svg">
    </a>
    <a href="https://goreportcard.com/report/github.com/teramoby/speedle-plus">
        <img src="https://goreportcard.com/badge/github.com/teramoby/speedle-plus">
    </a>
    <a href="https://pkg.go.dev/github.com/teramoby/speedle-plus">
        <img src="https://pkg.go.dev/badge/github.com/teramoby/speedle-plus.svg">
    </a>
</p>

<p align="right">
<a href="README.zh-cn.md">中文版</a>
</p>

# Speedle+

Speedle+ is a general purpose authorization engine. It allows users to construct their policy model with user-friendly policy definition language and get authorization decision in milliseconds based on the policies. Speedle is very user-friendly, efficient, and extremely scalable. 

Speedle+ open source project consists of a policy definition language, policy management module, authorization runtime module, commandline tool, and integration samples with popular systems.

Speedle+ is based on Speedle open source project which is hosted at https://github.com/oracle/speedle under UPL.

## Who are we

We are the founding members of Speedle project. Now we are not Oracle employees and consequently not contributors of Speedle project on GitHub anymore. But we still stay with Speedle project. We create a new repo under https://github.com/teramoby/speedle-plus and maintain the new project here now.

## Architecture

Speedle+ consists of three main components that work together to provide policy-based authorization:

```
                    +-----------+
                    |   spctl   |  Command-line tool for managing policies
                    +-----+-----+
                          |
              +-----------+-----------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      |     PMS      |        |     ADS      |
      | Policy Mgmt  |  push  |  Authz Check |
      |   Service    +------->+   Service    |
      +-------+------+        +-------+------+
              |                       |
              v                       v
      +-------+------+        +-------+------+
      | Policy Store |        | Policy Store |
      | (file/etcd/  |<-------+ (read-only)  |
      |  mongodb)    |  watch |              |
      +--------------+        +--------------+
```

- **spctl**: Command-line interface for creating, reading, updating, and deleting policies, services, and functions. Communicates with PMS via REST API.
- **PMS (Policy Management Service)**: Provides the REST/gRPC API for policy lifecycle management. Writes policies to the backing store.
- **ADS (Authorization Decision Service)**: Evaluates authorization requests in real time. Loads policies into an in-memory cache and watches the store for changes.

## PDL Syntax Examples

Speedle uses a human-readable Policy Definition Language (PDL). Here are basic examples:

**Define a service:**
```pdl
service bookstore {
    // Grant policy: allow users in group "employee" to read any book
    grant policy readBooks {
        principals: ["group:employee"]
        permissions: [
            {resource: "book", actions: ["read"]}
        ]
    }

    // Deny policy: deny user "bob" from deleting books
    deny policy bobCannotDelete {
        principals: ["user:bob"]
        permissions: [
            {resource: "book", actions: ["delete"]}
        ]
    }

    // Role policy: grant "reader" role to group "intern"
    grant role policy readerRole {
        principals: ["group:intern"]
        roles: ["reader"]
        resources: ["book"]
    }

    // Policy with condition expression
    grant policy weekdayAccess {
        principals: ["user:*"]
        permissions: [
            {resource: "book", actions: ["read"]}
        ]
        condition: request_weekday != "Saturday" && request_weekday != "Sunday"
    }
}
```

## Configuration

Speedle uses a JSON configuration file. Example:

```json
{
    "storeConfig": {
        "storeType": "file",
        "storeProps": {
            "FileLocation": "/etc/speedle/policies.json"
        }
    },
    "enableWatch": true,
    "serverConfig": {
        "endpoint": "0.0.0.0:6733",
        "insecure": "true"
    },
    "logConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/speedle.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 30
        }
    },
    "auditLogConfig": {
        "level": "info",
        "formatter": "json",
        "rotationConfig": {
            "filename": "/var/log/speedle/audit.log",
            "maxSize": 10,
            "maxBackups": 5,
            "maxAge": 90
        }
    }
}
```

## Documentation

- Latest documentations: <https://speedle.io/docs>
- Go package reference: <https://pkg.go.dev/github.com/teramoby/speedle-plus>

## Get Started

See Getting Started at <https://speedle.io/quick-start/>.

## Build

### Prerequisites

- Go 1.22 or greater <https://go.dev/doc/install>

### Step

```
$ go install github.com/teramoby/speedle-plus/cmd/spctl@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-ads@latest
$ go install github.com/teramoby/speedle-plus/cmd/speedle-pms@latest
$ ls $(go env GOPATH)/bin
spctl  speedle-ads  speedle-pms
```

Or clone the repository and build from source:

```
$ git clone https://github.com/teramoby/speedle-plus.git
$ cd speedle-plus
$ make build
```

## Test

```
$ cd speedle-plus
$ make test
```

## Security

**Important:** The PMS (Policy Management Service) management endpoints have no built-in authentication.
In production, bind the PMS server to localhost only and use a reverse proxy with authentication:

```
$ speedle-pms --endpoint=127.0.0.1:6733
```

Alternatively, run the PMS behind a firewall that restricts access to authorized administrators only.
Exposing PMS management endpoints on a public network interface without authentication allows anyone
to create, modify, or delete all policies, services, and functions.

## Get Help

- Join us on Slack: [#speedle-users](https://join.slack.com/t/speedleproject/shared_invite/zt-72fgiyuo-QKJAhHAqVbn17KRFbd7aZw)
- Mailing List: speedle-users@googlegroups.com

## Get Involved

- Learn how to [contribute](CONTRIBUTING.md)
- See [issues](https://github.com/teramoby/speedle-plus/issues) for issues you can help with
