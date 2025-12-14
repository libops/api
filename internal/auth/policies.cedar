// Policies for LibOps Authorization

// Owner Role grants all permissions on the resource
permit(
    principal, 
    action, 
    resource
)
when {
    principal in resource.owner_role
};

// Developer Role grants Read and Write permissions
permit(
    principal, 
    action in [Action::"read", Action::"write"], 
    resource
)
when {
    principal in resource.developer_role
};

// Viewer Role grants Read permission
permit(
    principal, 
    action == Action::"read", 
    resource
)
when {
    principal in resource.viewer_role
};
