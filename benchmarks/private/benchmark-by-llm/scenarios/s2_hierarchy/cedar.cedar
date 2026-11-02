permit(principal in Role::"viewer", action == Action::"read", resource == Resource::"/docs");
permit(principal in Role::"editor", action == Action::"write", resource == Resource::"/docs");
