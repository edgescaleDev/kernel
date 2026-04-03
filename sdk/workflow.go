package sdk

// WorkflowRegistry allows services to register Temporal workflows.
// The kernel collects these registrations and binds them to the Temporal worker.
type WorkflowRegistry interface {
	// RegisterWorkflow registers a workflow function with the Temporal worker.
	RegisterWorkflow(workflow any)

	// RegisterWorkflowWithOptions registers a workflow with custom options.
	RegisterWorkflowWithOptions(workflow any, options WorkflowOptions)
}

// WorkflowOptions configures workflow registration.
type WorkflowOptions struct {
	// Name overrides the default workflow type name.
	Name string
}

// ActivityRegistry allows services to register Temporal activities.
// The kernel collects these registrations and binds them to the Temporal worker.
type ActivityRegistry interface {
	// RegisterActivity registers an activity function or struct.
	RegisterActivity(activity any)

	// RegisterActivityWithOptions registers an activity with custom options.
	RegisterActivityWithOptions(activity any, options ActivityOptions)
}

// ActivityOptions configures activity registration.
type ActivityOptions struct {
	// Name overrides the default activity type name.
	Name string
}
