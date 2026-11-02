permit(principal in Role::"manager", action == Action::"approve", resource == Resource::"/expense") when { context.amount <= 1000 };
permit(principal in Role::"director", action == Action::"approve", resource == Resource::"/expense") when { context.amount <= 100000 };
