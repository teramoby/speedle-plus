permit(principal, action == Action::"edit", resource) when { context.owner == principal && context.path like "/docs/*" };
