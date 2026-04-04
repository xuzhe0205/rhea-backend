package rag

type Scope string

const (
	ScopeConversationOnly       Scope = "conversation_only"
	ScopeConversationAndProject Scope = "conversation_and_project"
	ScopeProjectOnly            Scope = "project_only"
)
