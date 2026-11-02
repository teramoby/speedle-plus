permit(principal in Role::"staff", action == Action::"read", resource) when { context.path like "/hr/*" };
forbid(principal == User::"kim", action == Action::"read", resource == Resource::"/hr/salaries");
