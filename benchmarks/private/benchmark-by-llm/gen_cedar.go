package authzbench

import (
	"bufio"
	"fmt"
)

// Cedar has two natural ways to hold N permissions, and both are measured:
//
//	cedar       one permit per permission. Cedar evaluates every policy that
//	            could apply, so the cost grows with the policy count.
//	cedar-data  one policy for the whole set; the permissions are entity
//	            data (resource.readers), which is how Cedar's documentation
//	            suggests modelling large, data-driven grants.
//
// Both use the same users: User entities whose parents are their Role.
// There is no data-driven wildcard variant: Cedar has no prefix lookup, so
// it would need a per-request resource hierarchy instead.

// cedarGrant is role r reading one resource (or a resource prefix).
type cedarGrant struct {
	role     int
	resource string
}

func writeCedar(g *GenSet, base string, grants []cedarGrant, members func(w *bufio.Writer, entity func(id string, role int))) error {
	g.CedarPolicies, g.CedarEntities = base+"_cedar.cedar", base+"_cedar_entities.json"
	condition := ""
	if g.Kind == "abac" {
		condition = `context.att1 == "val1" && context.att2 == "val2"`
	}
	err := writeFile(g.CedarPolicies, func(w *bufio.Writer) {
		for _, gr := range grants {
			switch {
			case g.Kind == "wildcard":
				fmt.Fprintf(w, "permit(principal in Role::\"role%d\", action == Action::\"read\", resource) when { context.path like \"%s\" };\n", gr.role, gr.resource)
			case condition != "":
				fmt.Fprintf(w, "permit(principal in Role::\"role%d\", action == Action::\"read\", resource == Resource::\"%s\") when { %s };\n", gr.role, gr.resource, condition)
			default:
				fmt.Fprintf(w, "permit(principal in Role::\"role%d\", action == Action::\"read\", resource == Resource::\"%s\");\n", gr.role, gr.resource)
			}
		}
	})
	if err != nil {
		return err
	}
	users := func(w *bufio.Writer) {
		first := true
		members(w, func(id string, role int) {
			if !first {
				w.WriteByte(',')
			}
			first = false
			fmt.Fprintf(w, "\n{\"uid\":{\"type\":\"User\",\"id\":%q},\"attrs\":{},\"parents\":[{\"type\":\"Role\",\"id\":\"role%d\"}]}", id, role)
		})
	}
	if err := writeFile(g.CedarEntities, func(w *bufio.Writer) { w.WriteByte('['); users(w); w.WriteString("\n]\n") }); err != nil {
		return err
	}
	if g.Kind == "wildcard" {
		return nil
	}

	g.CedarDataPolicies, g.CedarDataEntities = base+"_cedar_data.cedar", base+"_cedar_data_entities.json"
	when := "principal in resource.readers"
	if condition != "" {
		when += " && " + condition
	}
	policy := fmt.Sprintf("permit(principal, action == Action::\"read\", resource) when { %s };\n", when)
	if err := writeFile(g.CedarDataPolicies, func(w *bufio.Writer) { w.WriteString(policy) }); err != nil {
		return err
	}
	readers := map[string][]int{}
	var order []string
	for _, gr := range grants {
		if _, seen := readers[gr.resource]; !seen {
			order = append(order, gr.resource)
		}
		readers[gr.resource] = append(readers[gr.resource], gr.role)
	}
	return writeFile(g.CedarDataEntities, func(w *bufio.Writer) {
		w.WriteByte('[')
		users(w)
		for _, res := range order {
			fmt.Fprintf(w, ",\n{\"uid\":{\"type\":\"Resource\",\"id\":%q},\"attrs\":{\"readers\":[", res)
			for i, role := range readers[res] {
				if i > 0 {
					w.WriteByte(',')
				}
				fmt.Fprintf(w, "{\"__entity\":{\"type\":\"Role\",\"id\":\"role%d\"}}", role)
			}
			w.WriteString("]},\"parents\":[]}")
		}
		w.WriteString("\n]\n")
	})
}
