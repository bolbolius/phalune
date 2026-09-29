package launcher

import (
	"math"
	"sort"
	"strings"
)

func FuzzyScore(needle, haystack string) int {
	ni, hi := 0, 0
	consecutive := 0
	quality := 0

	for ni < len(needle) && hi < len(haystack) {
		if needle[ni] == haystack[hi] {
			ni++
			consecutive++
			quality += consecutive * 2
		} else {
			consecutive = 0
		}
		hi++
	}

	if ni < len(needle) {
		return 0
	}

	maxQuality := len(needle) * len(needle) * 2
	score := int(math.Round((float64(quality) / float64(maxQuality)) * 100.0))
	if score < 1 {
		return 1
	}
	return score
}

type scoredApp struct {
	app   App
	score float64
}

type Result struct {
	App     *App
	Action  *DesktopAction
	Command *commandEntry
}

func (r Result) Title() string {
	switch {
	case r.Command != nil:
		return ":" + r.Command.Name
	case r.Action != nil:
		return r.Action.Name
	case r.App != nil:
		return r.App.Name
	}
	return ""
}

func (r Result) Subtitle() string {
	switch {
	case r.Command != nil:
		return r.Command.Description
	case r.Action != nil:
		if r.App != nil {
			return r.App.Name
		}
	case r.App != nil:
		if r.App.GenericName != "" {
			return r.App.GenericName
		}
		return r.App.Comment
	}
	return ""
}

func (r Result) IconName() string {
	switch {
	case r.Command != nil:
		if r.Command.Icon != "" {
			return r.Command.Icon
		}
	case r.Action != nil:
		if r.Action.Icon != "" {
			return r.Action.Icon
		}
		if r.App != nil && r.App.Icon != "" {
			return r.App.Icon
		}
	case r.App != nil && r.App.Icon != "":
		return r.App.Icon
	}
	return "application-x-executable"
}

func SortAppsAlphabetical(apps []App) []App {
	result := make([]App, len(apps))
	copy(result, apps)
	sort.Slice(result, func(i, j int) bool {
		return strings.ToLower(result[i].Name) < strings.ToLower(result[j].Name)
	})
	return result
}

func SortAppsHybrid(apps []App, store *FrecencyStore) []App {
	if len(apps) == 0 {
		return nil
	}

	var frequent []scoredApp
	var rest []App

	for _, app := range apps {
		score := 0.0
		if store != nil {
			score = store.Score(app.ID)
		}

		if score >= frecencyThreshold {
			frequent = append(frequent, scoredApp{app: app, score: score})
		} else {
			rest = append(rest, app)
		}
	}

	sort.Slice(frequent, func(i, j int) bool {
		if frequent[i].score != frequent[j].score {
			return frequent[i].score > frequent[j].score
		}
		return strings.ToLower(frequent[i].app.Name) < strings.ToLower(frequent[j].app.Name)
	})

	sort.Slice(rest, func(i, j int) bool {
		return strings.ToLower(rest[i].Name) < strings.ToLower(rest[j].Name)
	})

	result := make([]App, 0, len(apps))
	for _, fa := range frequent {
		result = append(result, fa.app)
	}
	result = append(result, rest...)
	return result
}

func FilterApps(apps []App, query string, store *FrecencyStore) []App {
	if len(apps) == 0 {
		return nil
	}

	qTrimmed := strings.ToLower(strings.TrimSpace(query))
	if qTrimmed == "" {
		return SortAppsHybrid(apps, store)
	}

	qCompact := strings.ReplaceAll(qTrimmed, " ", "")
	var scored []scoredApp

	for _, app := range apps {
		name := strings.ToLower(app.Name)
		id := strings.ToLower(app.ID)
		generic := strings.ToLower(app.GenericName)
		comment := strings.ToLower(app.Comment)
		execStr := strings.ToLower(app.CleanExec)
		if execStr == "" {
			execStr = strings.ToLower(app.Exec)
		}
		keywords := strings.ToLower(strings.Join(app.Keywords, " "))
		cats := strings.ToLower(strings.Join(app.Categories, " "))

		nameC := strings.ReplaceAll(name, " ", "")
		idC := strings.ReplaceAll(id, " ", "")
		genericC := strings.ReplaceAll(generic, " ", "")
		execC := strings.ReplaceAll(execStr, " ", "")
		keywordsC := strings.ReplaceAll(keywords, " ", "")
		catsC := strings.ReplaceAll(cats, " ", "")
		commentC := strings.ReplaceAll(comment, " ", "")

		matchScore := 0.0

		if nameC == qCompact {
			matchScore = 100
		} else if idC == qCompact {
			matchScore = 95
		} else if strings.HasPrefix(nameC, qCompact) {
			matchScore = 80
		} else if strings.Contains(name, " "+qTrimmed) || strings.Contains(name, "-"+qTrimmed) {
			matchScore = 70
		} else if strings.HasPrefix(genericC, qCompact) {
			matchScore = 65
		} else if strings.HasPrefix(idC, qCompact) {
			matchScore = 60
		} else if strings.Contains(nameC, qCompact) {
			matchScore = 50
		} else if strings.Contains(genericC, qCompact) {
			matchScore = 45
		} else if strings.Contains(keywordsC, qCompact) {
			matchScore = 40
		} else if strings.HasPrefix(execC, qCompact) {
			matchScore = 35
		} else if strings.Contains(catsC, qCompact) {
			matchScore = 30
		} else if strings.Contains(commentC, qCompact) {
			matchScore = 20
		} else if strings.Contains(execC, qCompact) {
			matchScore = 10
		} else if len(qCompact) >= 2 {
			fzName := FuzzyScore(qCompact, nameC)
			if fzName > 0 {
				matchScore = 5.0 + math.Round((float64(fzName)/100.0)*13.0)
			} else {
				fzId := FuzzyScore(qCompact, idC)
				if fzId > 0 {
					matchScore = 5.0 + math.Round((float64(fzId)/100.0)*10.0)
				} else {
					fzGeneric := FuzzyScore(qCompact, genericC)
					if fzGeneric > 0 {
						matchScore = 5.0 + math.Round((float64(fzGeneric)/100.0)*8.0)
					}
				}
			}
		}

		if matchScore > 0 {
			bonus := 0.0
			if store != nil {
				fScore := store.Score(app.ID)
				if fScore >= frecencyThreshold {
					bonus = math.Min(store.MaxBoost(), math.Log(1.0+fScore)*4.0)
				}
			}

			scored = append(scored, scoredApp{
				app:   app,
				score: matchScore + bonus,
			})
		}
	}

	sort.Slice(scored, func(i, j int) bool {
		if scored[i].score != scored[j].score {
			return scored[i].score > scored[j].score
		}
		return strings.ToLower(scored[i].app.Name) < strings.ToLower(scored[j].app.Name)
	})

	result := make([]App, len(scored))
	for i, s := range scored {
		result[i] = s.app
	}
	return result
}

func FilterResults(apps []App, commands []commandEntry, query string, store *FrecencyStore) []Result {
	q := strings.TrimSpace(query)
	isCommandQuery := strings.HasPrefix(q, ":")
	qLower := strings.ToLower(q)

	type scoredResult struct {
		res   Result
		score float64
	}
	var scored []scoredResult

	if isCommandQuery {
		qCmd := strings.TrimPrefix(qLower, ":")
		for i := range commands {
			s := matchCommand(commands[i], qCmd)
			if s > 0 {
				scored = append(scored, scoredResult{
					res:   Result{Command: &commands[i]},
					score: float64(s) + 100,
				})
			}
		}
	} else {
		for i := range apps {
			for j := range apps[i].Actions {
				action := &apps[i].Actions[j]
				score := scoreAction(apps[i], *action, qLower)
				if score > 0 {
					scored = append(scored, scoredResult{
						res:   Result{App: &apps[i], Action: action},
						score: score,
					})
				}
			}
		}

		filtered := FilterApps(apps, q, store)
		for i := range filtered {
			scored = append(scored, scoredResult{
				res:   Result{App: &filtered[i]},
				score: 0,
			})
		}
	}

	if len(scored) == 0 {
		return nil
	}

	sort.SliceStable(scored, func(i, j int) bool {
		return scored[i].score > scored[j].score
	})

	results := make([]Result, 0, len(scored))
	for _, s := range scored {
		results = append(results, s.res)
	}
	return results
}

func scoreAction(app App, action DesktopAction, qLower string) float64 {
	if qLower == "" {
		return 0
	}
	q := strings.ReplaceAll(qLower, " ", "")

	actionName := strings.ToLower(action.Name)
	actionCompact := strings.ReplaceAll(actionName, " ", "")
	appName := strings.ToLower(app.Name)

	score := 0.0
	switch {
	case actionCompact == q:
		score = 92
	case strings.HasPrefix(actionCompact, q):
		score = 78
	case strings.Contains(actionCompact, q):
		score = 60
	case strings.Contains(q, appName) && appName != "":
		if az := FuzzyScore(strings.ReplaceAll(q, appName+" ", ""), actionCompact); az > 0 {
			score = 55 + float64(az)/10
		}
	}

	if score == 0 {
		combined := strings.ReplaceAll(appName+" "+actionName, " ", "")
		if fz := FuzzyScore(q, combined); fz > 0 && len(q) >= 3 {
			score = 40 + float64(fz)/10
		}
	}

	return score
}
