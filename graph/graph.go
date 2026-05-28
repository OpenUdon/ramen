package graph

import (
	"fmt"
	"slices"
)

type Node struct {
	Address   string
	DependsOn []string
}

func Sort(nodes []Node) ([]Node, error) {
	byAddress := map[string]Node{}
	for _, node := range nodes {
		if node.Address == "" {
			continue
		}
		node.DependsOn = uniqueSorted(node.DependsOn)
		byAddress[node.Address] = node
	}
	addresses := make([]string, 0, len(byAddress))
	for address := range byAddress {
		addresses = append(addresses, address)
	}
	slices.Sort(addresses)

	temporary := map[string]bool{}
	permanent := map[string]bool{}
	var out []Node
	var visit func(string) error
	visit = func(address string) error {
		if permanent[address] {
			return nil
		}
		if temporary[address] {
			return fmt.Errorf("dependency cycle includes %s", address)
		}
		node, ok := byAddress[address]
		if !ok {
			return nil
		}
		temporary[address] = true
		for _, dep := range node.DependsOn {
			if _, ok := byAddress[dep]; ok {
				if err := visit(dep); err != nil {
					return err
				}
			}
		}
		temporary[address] = false
		permanent[address] = true
		out = append(out, node)
		return nil
	}
	for _, address := range addresses {
		if err := visit(address); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func uniqueSorted(values []string) []string {
	seen := map[string]bool{}
	var out []string
	for _, value := range values {
		if value == "" || seen[value] {
			continue
		}
		seen[value] = true
		out = append(out, value)
	}
	slices.Sort(out)
	return out
}
