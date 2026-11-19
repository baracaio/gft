package handler

import (
	"context"
	"sort"
	"sync"

	"github.com/aws/aws-lambda-go/events"
)

func (h *Handler) processGroups(ctx context.Context, msgs []parsedMsg) []events.SQSBatchItemFailure {
	groups := groupByClient(msgs)
	for k := range groups {
		sortGroup(groups[k])
	}

	sem := make(chan struct{}, h.workers)
	var wg sync.WaitGroup
	var mu sync.Mutex
	var failures []events.SQSBatchItemFailure

	for _, group := range groups {
		sem <- struct{}{}
		wg.Add(1)
		go func(g []parsedMsg) {
			defer func() { <-sem; wg.Done() }()
			for _, m := range g {
				if err := h.save(ctx, m); err != nil {
					mu.Lock()
					failures = append(failures, events.SQSBatchItemFailure{ItemIdentifier: m.sqsID})
					mu.Unlock()
					if m.isLastAttempt() {
						_ = h.repo.SaveFailure(ctx, h.buildFailureRecord(&m, err))
					}
				}
			}
		}(group)
	}

	wg.Wait()
	return failures
}

func groupByClient(msgs []parsedMsg) map[string][]parsedMsg {
	groups := make(map[string][]parsedMsg, len(msgs))
	for _, m := range msgs {
		groups[m.clientCode] = append(groups[m.clientCode], m)
	}
	return groups
}

func sortGroup(group []parsedMsg) {
	sort.Slice(group, func(i, j int) bool {
		return group[i].updatedAt.Before(group[j].updatedAt)
	})
}
