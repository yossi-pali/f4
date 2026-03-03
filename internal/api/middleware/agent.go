package middleware

import (
	"context"
	"net/http"
	"strconv"
	"strings"

	"github.com/12go/f4/internal/domain"
)

type agentCtxKey struct{}

// AgentExtraction extracts agent context from the request.
func AgentExtraction(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		agent := domain.AgentContext{
			Referer: r.Header.Get("Referer"),
		}

		// Extract agent ID from header or query param.
		// PHP AuthenticationListener uses $request->get('a') for agent init.
		if idStr := r.Header.Get("X-Agent-ID"); idStr != "" {
			agent.AgentID, _ = strconv.Atoi(idStr)
		} else if idStr := r.URL.Query().Get("a"); idStr != "" {
			agent.AgentID, _ = strconv.Atoi(idStr)
		} else if idStr := r.URL.Query().Get("agent_id"); idStr != "" {
			agent.AgentID, _ = strconv.Atoi(idStr)
		}

		// API key
		agent.APIKey = r.Header.Get("X-API-Key")

		// Role
		agent.Role = r.Header.Get("X-Agent-Role")
		agent.IsAdmin = agent.Role == "admin"

		// Bot detection (simple user-agent check)
		ua := strings.ToLower(r.UserAgent())
		agent.IsBot = strings.Contains(ua, "bot") ||
			strings.Contains(ua, "crawler") ||
			strings.Contains(ua, "spider") ||
			strings.Contains(ua, "googlebot") ||
			strings.Contains(ua, "bingbot")

		ctx := context.WithValue(r.Context(), agentCtxKey{}, agent)
		next.ServeHTTP(w, r.WithContext(ctx))
	})
}

// AgentFromContext retrieves the agent context from a request context.
func AgentFromContext(ctx context.Context) domain.AgentContext {
	agent, _ := ctx.Value(agentCtxKey{}).(domain.AgentContext)
	return agent
}
