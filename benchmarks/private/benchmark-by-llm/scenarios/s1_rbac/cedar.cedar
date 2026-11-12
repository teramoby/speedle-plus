permit(principal in Role::"admin", action in [Action::"read", Action::"write"], resource == Resource::"/data1");
permit(principal in Role::"viewer", action == Action::"read", resource == Resource::"/data1");
