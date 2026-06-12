package mcpv2

import (
	"encoding/json"
	"fmt"
	"io"
	"log"
	"os"
	"runtime"
	"sort"
	"strings"
	"sync"
	"time"

	"github.com/AlHarisTech/ai-workstation-core/runtime/metrics"
)

type TokenBucket struct {
	mu         sync.Mutex
	tokens     float64
	maxTokens  float64
	refillRate float64
	lastRefill time.Time
}

func NewTokenBucket(maxTokens, refillRate float64) *TokenBucket {
	return &TokenBucket{
		tokens:     maxTokens,
		maxTokens:  maxTokens,
		refillRate: refillRate,
		lastRefill: time.Now(),
	}
}

func (tb *TokenBucket) TryTake() bool {
	tb.mu.Lock()
	defer tb.mu.Unlock()
	now := time.Now()
	elapsed := now.Sub(tb.lastRefill).Seconds()
	tb.tokens += elapsed * tb.refillRate
	if tb.tokens > tb.maxTokens {
		tb.tokens = tb.maxTokens
	}
	tb.lastRefill = now
	if tb.tokens >= 1 {
		tb.tokens--
		return true
	}
	return false
}

type Gateway struct {
	router              *Router
	policy              *PolicyEngine
	servers             map[string]MCPServer
	learningEngine      *LearningEngine
	exploration         *ExplorationState
	stability           *StabilityEngine
	enforcement         *EnforcementEngine
	policyIntelligence  *PolicyIntelligenceEngine
	rateLimiter         *TokenBucket
}

func NewGateway() *Gateway {
	g := &Gateway{
		router:              NewRouter(),
		policy:              NewPolicyEngine(),
		servers:             make(map[string]MCPServer),
		learningEngine:      NewLearningEngine(),
		exploration:         NewExplorationState(0.10),
		stability:           NewStabilityEngine(0.02, 20),
		enforcement:         NewEnforcementEngine(),
		policyIntelligence:  NewPolicyIntelligenceEngine(),
		rateLimiter:         NewTokenBucket(10000, 5000),
	}
	g.registerDefaults()
	return g
}

func (g *Gateway) registerDefaults() {
	g.servers["git"] = &GitServer{}
	g.servers["filesystem"] = &FilesystemServer{}
	g.servers["memory"] = NewMemoryServer("")
	g.servers["github"] = NewGitHubServer()
	g.servers["fetch"] = &FetchServer{}
	g.servers["context7"] = NewContext7Server()
	g.servers["postgres"] = NewPostgresAdapter()
	g.servers["chroma"] = NewChromaAdapter()
	g.servers["chromadb"] = g.servers["chroma"]
}

func (g *Gateway) Server(name string) MCPServer {
	if s, ok := g.servers[name]; ok {
		return s
	}
	return nil
}

func (g *Gateway) RegisterServer(s MCPServer) {
	g.servers[s.Name()] = s
}

func (g *Gateway) Process(req *MCPRequest) (resp *MCPResponse) {
	start := time.Now()
	// Ensure trace IDs are available before any processing
	if req.Meta.TraceID == "" {
		req.Meta.TraceID = GenerateTraceID()
	}
	if req.Meta.SpanID == "" {
		req.Meta.SpanID = GenerateSpanID()
	}

	resp = &MCPResponse{
		ID:        req.ID,
		RequestID: req.ID,
		Status:    "success",
		Execution: ExecutionResult{
			Operation: string(req.Action.Type) + "." + req.Action.Operation,
		},
		Result: ResultData{Format: "json"},
		Meta: ResponseMeta{
			TraceID: req.Meta.TraceID,
			SpanID:  req.Meta.SpanID,
		},
	}

	// Initialize decision trace (purely additive — no behavior change)
	trace := &DecisionTrace{
		TraceID:   req.Meta.TraceID,
		RequestID: req.ID,
	}

	// Stage 7: Learning feedback + Stability update + Governance audit (deferred — always runs on return)
	var auditServer string
	var auditOp string
	var auditExecutionAllowed = true
	var auditBlockReason string
	defer func() {
		if r := recover(); r != nil {
			resp.Status = "error"
			resp.Error = ErrorInfo{Code: "INTERNAL_PANIC", Message: fmt.Sprintf("panic: %v", r), Recoverable: false}
			trace.Steps = append(trace.Steps, TraceStep{Stage: "panic", Output: "recovered", Meta: map[string]any{"panic": fmt.Sprintf("%v", r)}})
			metrics.Global().RecordPanic()
			stack := make([]byte, 4096)
			n := runtime.Stack(stack, false)
			log.Printf("[gateway] PANIC RECOVERED: %v\n%s", r, stack[:n])
		}

		if auditServer != "" {
			if g.learningEngine != nil {
				g.learningEngine.Update(RoutingOutcome{
					RequestID:      req.ID,
					SelectedServer: auditServer,
					Success:        resp.Status == "success",
					LatencyMs:      time.Since(start).Milliseconds(),
					Timestamp:      time.Now(),
				})
			}
			if g.stability != nil {
				g.stability.RecordSelection(auditOp, auditServer)
			}
		}

		LogAudit(AuditRecord{
			Timestamp:        time.Now().UTC().Format(time.RFC3339),
			RequestID:        req.ID,
			TraceID:          req.Meta.TraceID,
			Action:           string(req.Action.Type) + "." + req.Action.Operation,
			Server:           auditServer,
			Status:           resp.Status,
			DurationMs:       time.Since(start).Milliseconds(),
			KnowledgeCount:   len(req.Context.Knowledge),
			ExecutionAllowed: auditExecutionAllowed,
			BlockReason:      auditBlockReason,
		})

		// Attach trace only at the end — zero impact on routing
		resp.Meta.DecisionTrace = trace
	}()

	// Stage 0: Rate Limiter (token bucket burst control)
	if !g.rateLimiter.TryTake() {
		metrics.Global().RecordRateLimit()
		trace.Steps = append(trace.Steps, TraceStep{Stage: "rate_limit", Output: "blocked"})
		return errorResponse(resp, "RATE_LIMITED", "rate limit exceeded", false)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "rate_limit", Output: "allowed"})

	// Stage 1: Validate request schema
	if err := ValidateRequest(req); err != nil {
		trace.Steps = append(trace.Steps, TraceStep{Stage: "validate", Output: "denied", Meta: map[string]any{"error": err.Error()}})
		trace.SelectedServer = ""
		return errorResponse(resp, "VALIDATION_ERROR", err.Error(), false)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "validate", Output: "ok"})

	// Stage 2: Enforce policy (fail closed)
	if err := g.policy.Enforce(req.Action.Type, req.Action.Operation, req.Policy); err != nil {
		trace.Steps = append(trace.Steps, TraceStep{Stage: "policy", Output: "denied", Meta: map[string]any{"reason": err.Error()}})
		return errorResponse(resp, "POLICY_DENIED", err.Error(), false)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "policy", Output: "allow"})

	// Stage 3: Resolve capability → server mapping
	cap, err := g.router.Resolve(req.Action.Type, req.Action.Operation)
	if err != nil {
		trace.Steps = append(trace.Steps, TraceStep{Stage: "resolve", Output: "not_found", Meta: map[string]any{"error": err.Error()}})
		return errorResponse(resp, "ROUTE_NOT_FOUND", err.Error(), false)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "resolve", Output: cap.Server, Meta: map[string]any{"capabilities": cap.Capabilities}})

	// Stage 4: Knowledge retrieval (non-blocking, best-effort)
	if ka := g.Server("chroma"); ka != nil {
		if chroma, ok := ka.(*ChromaAdapter); ok {
			kStart := time.Now()
			kQuery := string(req.Action.Type) + "." + req.Action.Operation
			kResults, kErr := chroma.QueryKnowledge("", kQuery)
			if kErr != nil {
				trace.Steps = append(trace.Steps, TraceStep{Stage: "knowledge", Output: "failed", Meta: map[string]any{"error": kErr.Error(), "query": kQuery}})
				log.Printf("[gateway] knowledge retrieval failed (non-blocking): %v", kErr)
			} else {
				req.Context.Knowledge = append(req.Context.Knowledge, KnowledgeDoc{
					Collection: "mcp_execution_memory",
					Query:      kQuery,
					Results:    kResults,
					DurationMs: time.Since(kStart).Milliseconds(),
				})
				docCount := 0
				if results, ok := kResults.(map[string]any); ok {
					if docs, ok := results["documents"]; ok {
						if arr, ok := docs.([]any); ok {
							docCount = len(arr)
						}
					}
				}
				kw := []string{}
				for _, kd := range req.Context.Knowledge {
					kw = append(kw, kd.Collection+":"+kd.Query)
				}
				trace.KnowledgeUsed = kw
				trace.Steps = append(trace.Steps, TraceStep{Stage: "knowledge", Output: fmt.Sprintf("%d_docs", docCount), Meta: map[string]any{"query": kQuery, "docs": docCount}})
			}
		}
	}

	// Stage 4.5: Knowledge-driven server selection
	if len(req.Context.Knowledge) > 0 {
		candidates := g.router.ListAll()
		scored := g.selectBestServer(candidates, req, req.Context.Knowledge, trace)
		if scored != nil && scored.Server != cap.Server {
			defCap, defKW, defHist := g.learningEngine.WeightsFor(cap.Server).Factors()
			scoredCap, scoredKW, scoredHist := g.learningEngine.WeightsFor(scored.Server).Factors()
			defaultScore := scoreCapability(req, cap, req.Context.Knowledge, defCap, defKW, defHist)
			scoredScore := scoreCapability(req, scored, req.Context.Knowledge, scoredCap, scoredKW, scoredHist)
			op := string(req.Action.Operation)
			if g.exploration != nil {
				effDef := g.exploration.ExplorationRate
				effScored := g.exploration.ExplorationRate
				if g.stability != nil {
					effDef = g.stability.EffectiveRate(cap.Server, effDef)
					effScored = g.stability.EffectiveRate(scored.Server, effScored)
				}
				defaultScore = g.exploration.AdjustScoreWithRate(cap.Server, defaultScore, effDef)
				scoredScore = g.exploration.AdjustScoreWithRate(scored.Server, scoredScore, effScored)
			}
			if g.stability != nil {
				defaultScore = g.stability.AdjustScore(cap.Server, op, defaultScore)
				scoredScore = g.stability.AdjustScore(scored.Server, op, scoredScore)
			}
			if scoredScore > defaultScore {
				oscMsg := ""
				if g.stability != nil {
					oscMsg = fmt.Sprintf(" osc=%d cvg=%.2f", g.stability.OscillationCount(op), g.stability.ConvergenceScore(op))
				}
				log.Printf("[gateway] routing override: %s(%.2f) → %s(%.2f) knowledge-driven%s",
					cap.Server, defaultScore, scored.Server, scoredScore, oscMsg)
				trace.Steps = append(trace.Steps, TraceStep{Stage: "override", Output: fmt.Sprintf("%s→%s", cap.Server, scored.Server), Meta: map[string]any{"from_score": defaultScore, "to_score": scoredScore}})
				cap = scored
			}
		}
	}

	// Stage 5: Route to MCP server
	server, ok := g.servers[cap.Server]
	if !ok {
		trace.Steps = append(trace.Steps, TraceStep{Stage: "route", Output: "not_found", Meta: map[string]any{"server": cap.Server}})
		return errorResponse(resp, "SERVER_NOT_FOUND", "no server registered: "+cap.Server, false)
	}
	trace.SelectedServer = cap.Server
	trace.Steps = append(trace.Steps, TraceStep{Stage: "route", Output: cap.Server})

	resp.Execution.Server = cap.Server
	auditServer = cap.Server
	auditOp = string(req.Action.Operation)
	if g.exploration != nil {
		g.exploration.RecordSelection(cap.Server)
	}

	// Stage 5.5: Enforcement Gate — control plane check before execution
	fullOp := string(req.Action.Type) + "." + req.Action.Operation
	enforceResult := EnforcementResult{Allowed: true, Server: cap.Server, Operation: fullOp}
	if g.enforcement != nil {
		enforceResult = g.enforcement.Check(cap.Server, fullOp)
	}

	// Policy Intelligence — passive observer (no runtime impact)
	if g.policyIntelligence != nil {
		g.policyIntelligence.Record(PolicyEvent{
			TraceID:   req.Meta.TraceID,
			RequestID: req.ID,
			Server:    cap.Server,
			Operation: fullOp,
			Allowed:   enforceResult.Allowed,
			Blocked:   !enforceResult.Allowed,
			Reason:    enforceResult.BlockReason,
		})
	}

	if !enforceResult.Allowed {
		auditExecutionAllowed = false
		auditBlockReason = enforceResult.BlockReason
		trace.Steps = append(trace.Steps, TraceStep{Stage: "enforcement", Output: "blocked", Meta: map[string]any{"reason": enforceResult.BlockReason, "server": cap.Server}})
		log.Printf("[gateway] enforcement blocked: server=%s operation=%s reason=%s", cap.Server, fullOp, enforceResult.BlockReason)
		return errorResponse(resp, "ENFORCEMENT_BLOCKED", enforceResult.BlockReason, false)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "enforcement", Output: "allowed"})

	// Stage 6: Execute MCP call
	req.Context.TimeoutMs = req.Meta.Timeout
	result, execErr := server.Execute(fullOp, req.Payload.Parameters, req.Context)
	resp.Execution.Duration = time.Since(start).Milliseconds()

	if execErr != nil {
		trace.Steps = append(trace.Steps, TraceStep{Stage: "execute", Output: "failed", Meta: map[string]any{"error": execErr.Error()}})
		return errorResponse(resp, "EXECUTION_FAILED", execErr.Error(), true)
	}
	trace.Steps = append(trace.Steps, TraceStep{Stage: "execute", Output: "ok"})

	// Stage 8: Normalize response
	resp.Result.Data = result
	resp.Result.Format = "json"

	return resp
}

func errorResponse(resp *MCPResponse, code, message string, recoverable bool) *MCPResponse {
	resp.Status = "error"
	resp.Error = ErrorInfo{
		Code:        code,
		Message:     message,
		Recoverable: recoverable,
	}
	return resp
}

func flattenKnowledge(knowledge []KnowledgeDoc) string {
	var parts []string
	for _, doc := range knowledge {
		data, _ := json.Marshal(doc.Results)
		parts = append(parts, string(data))
	}
	return strings.Join(parts, " ")
}

func scoreCapability(req *MCPRequest, cap *Capability, knowledge []KnowledgeDoc, capW, kwW, histW float64) float64 {
	score := 0.0

	for _, op := range cap.Capabilities {
		if op == req.Action.Operation {
			score += capW
			break
		}
	}

	if len(knowledge) > 0 {
		kText := strings.ToLower(flattenKnowledge(knowledge))
		serverWords := strings.Fields(strings.ToLower(cap.Server + " " + strings.Join(cap.Capabilities, " ")))
		matches := 0
		for _, w := range serverWords {
			if len(w) > 2 && strings.Contains(kText, w) {
				matches++
			}
		}
		if len(serverWords) > 0 {
			ratio := float64(matches) / float64(len(serverWords))
			score += ratio * kwW
		}

		mentionCount := 0
		for _, doc := range knowledge {
			docStr := strings.ToLower(fmt.Sprintf("%v", doc.Results))
			if strings.Contains(docStr, strings.ToLower(cap.Server)) {
				mentionCount++
			}
		}
		if len(knowledge) > 0 {
			score += (float64(mentionCount) / float64(len(knowledge))) * histW
		}
	}

	return score
}

func (g *Gateway) selectBestServer(candidates []*Capability, req *MCPRequest, knowledge []KnowledgeDoc, trace *DecisionTrace) *Capability {
	type scoredCap struct {
		cap   *Capability
		score float64
		index int
	}

	op := string(req.Action.Operation)
	var scored []scoredCap
	for i, cap := range candidates {
		capW, kwW, histW := 0.30, 0.40, 0.30
		if g.learningEngine != nil {
			capW, kwW, histW = g.learningEngine.WeightsFor(cap.Server).Factors()
		}
		s := scoreCapability(req, cap, knowledge, capW, kwW, histW)
		if g.exploration != nil {
			effRate := g.exploration.ExplorationRate
			if g.stability != nil {
				effRate = g.stability.EffectiveRate(cap.Server, effRate)
			}
			s = g.exploration.AdjustScoreWithRate(cap.Server, s, effRate)
		}
		if g.stability != nil {
			s = g.stability.AdjustScore(cap.Server, op, s)
		}
		scored = append(scored, scoredCap{cap, s, i})
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return scored[i].index < scored[j].index
	})

	scores := make(map[string]float64)
	for _, s := range scored {
		scores[s.cap.Server] = s.score
	}
	selected := scored[0].cap

	if trace != nil {
		trace.ServerScores = scores
		if len(scored) > 1 {
			trace.SecondBest = scored[1].cap.Server
			trace.ScoreDelta = scored[0].score - scored[1].score
		}
	}

	scoreJSON, _ := json.Marshal(scores)
	expApplied := g.exploration != nil && g.exploration.ExplorationRate > 0
	expFactor := 0.0
	if expApplied {
		expFactor = g.exploration.ExplorationRate
	}
	osc, cvg := 0, 0.0
	if g.stability != nil {
		osc = g.stability.OscillationCount(op)
		cvg = g.stability.ConvergenceScore(op)
	}
	log.Printf(`[gateway] routing_mode=stable_adaptive selected_server=%s scores=%s exploration={"applied":%v,"factor":%.2f} stability={"oscillation_score":%d,"convergence_score":%.2f}`,
		selected.Server, string(scoreJSON), expApplied, expFactor, osc, cvg)

	return selected
}

func (g *Gateway) Listen() *MCPRequest {
	var req MCPRequest
	err := json.NewDecoder(os.Stdin).Decode(&req)
	if err == io.EOF {
		return nil
	}
	if err != nil {
		log.Printf("[gateway] failed to read request (returning nil): %v", err)
		return nil
	}
	return &req
}
