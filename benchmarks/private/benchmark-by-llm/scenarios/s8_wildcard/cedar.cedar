permit(principal in Role::"analyst", action == Action::"read", resource) when { context.path like "/reports/*" };
