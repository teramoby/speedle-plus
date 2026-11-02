permit(principal, action in [Action::"read", Action::"write"], resource == Resource::"/data") when { context.tenant in principal.adminOf };
permit(principal, action == Action::"read", resource == Resource::"/data") when { context.tenant in principal.viewerOf };
