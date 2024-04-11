package ecodisaster

import (
	"fmt"
	"strconv"
)

const (
	// Indices into various arrays.
	RED   = 0
	GREEN = 1

	// Number of barrels that the bot can carry at once.  We'll
	// make sure that these are always all the same colour.
	BOT_CAPACITY = 4
)

type coords struct {
	x, y int
}

func (c coords) String() string {
	return fmt.Sprintf("%v:%v", c.x, c.y)
}

type world struct {
	// Coordinates of barrels that still need to be collected.
	barrels [2][]coords

	// Number of barrels that the bot is already carrying.
	botLoad int

	// Colour of the barrels that the bot is already carrying;
	// only significant when botLoad > 0.
	botColour int
}

// The initial state, given specified barrel positions.
func initialState(barrels [2][]coords) *world {
	return &world{
		barrels: barrels,
		botLoad: 0,
	}
}

// The following state, if we decide to pick up (and hence remove) the
// Ith barrel of colour C.
func nextState(state *world, c int, i int) (*world, bool, bool) {
	next := &world{
		barrels:   [2][]coords{nil, nil},
		botColour: state.botColour,
	}
	dropOffNeededBeforeChoice := false

	// Copy remaining coordinates of the barrels whose colour we
	// are picking up.
	next.barrels[c] = make([]coords, len(state.barrels[c])-1)
	for j := 0; j < i; j++ {
		next.barrels[c][j] = state.barrels[c][j]
	}
	for j := i + 1; j < len(state.barrels[c]); j++ {
		next.barrels[c][j-1] = state.barrels[c][j]
	}

	// Copy coordinates of barrels of the other colour.  Here we
	// can reuse the slice, as we are treating it as immutable.
	otherColour := 1 - c
	next.barrels[otherColour] = state.barrels[otherColour]

	// Update what the bot is carrying and whether a drop-off is
	// needed.
	if state.botLoad == 0 {
		dropOffNeededBeforeChoice = false
		next.botLoad = 1
		next.botColour = c
	} else if c != state.botColour {
		// Change of colour.
		dropOffNeededBeforeChoice = true
		next.botLoad = 1
		next.botColour = c
	} else if state.botLoad == BOT_CAPACITY {
		// Bot was already full, even though the colour is not
		// changing.
		dropOffNeededBeforeChoice = true
		next.botLoad = 1
	} else {
		// Same colour and bot still has capacity.
		next.botLoad = state.botLoad + 1
	}

	return next, dropOffNeededBeforeChoice, len(next.barrels[0]) == 0 && len(next.barrels[1]) == 0
}

type choice struct {
	// The colour of the barrel that we choose to pick up next.
	colour int

	// The coordinates of the barrel.
	coords coords
}

type route struct {
	// The first choice that this route makes.
	first choice

	// The route following that first choice.
	next *route

	// The overall cost of this route.
	cost int
}

type botPosition struct {
	coords coords
	// Will probably later add orientation here.
}

var dropZone [2]*botPosition

var bestRouteCache = map[string]*route{}

func routeCacheKey(p *botPosition, state *world) string {
	k := fmt.Sprintf("%v-%v-%v-%v", p.coords, state.barrels[RED], state.barrels[GREEN], state.botLoad)
	if state.botLoad > 0 {
		k += fmt.Sprintf("-%v", state.botColour)
	}
	return k
}

// Given a bot position and a set of barrels that still need
// collecting, calculate and return the best route.
func bestRoute(p *botPosition, state *world) *route {
	cacheKey := routeCacheKey(p, state)
	cachedRoute := bestRouteCache[cacheKey]
	if cachedRoute != nil {
		return cachedRoute
	}
	minCost := 0
	var bestChoice *choice = nil
	var bestFollowingRoute *route
	for _, choiceColour := range []int{RED, GREEN} {
		for choiceIndex := range state.barrels[choiceColour] {
			choiceIndex := choiceIndex
			choicePosition := &botPosition{
				coords: state.barrels[choiceColour][choiceIndex],
			}
			followingState, dropOffNeededBeforeChoice, dropOffNeededAfterChoice := nextState(state, choiceColour, choiceIndex)
			followingBestRoute := bestRoute(choicePosition, followingState)

			// The bot is currently at P.  If a drop-off
			// is needed it must move:
			// 1. from P to the drop zone for state.botColour
			// 2. from the drop zone to choicePosition
			//    (the chosen next barrel)
			// 3. then follow the best route from
			//    choicePosition onwards.
			// If a drop-off is not needed it must move:
			// 1. from P to choicePosition
			// 2. then follow the best route from
			//    choicePosition onwards.
			var thisChoiceCost int
			if dropOffNeededBeforeChoice {
				thisChoiceCost = moveCost(p, dropZone[state.botColour]) +
					dropCost() +
					moveCost(dropZone[state.botColour], choicePosition)
			} else {
				thisChoiceCost = moveCost(p, choicePosition)
			}
			if dropOffNeededAfterChoice {
				thisChoiceCost += moveCost(choicePosition, dropZone[choiceColour]) + dropCost()
			}
			if followingBestRoute != nil {
				thisChoiceCost += followingBestRoute.cost
			}
			if bestChoice == nil || thisChoiceCost < minCost {
				minCost = thisChoiceCost
				bestChoice = &choice{
					colour: choiceColour,
					coords: state.barrels[choiceColour][choiceIndex],
				}
				bestFollowingRoute = followingBestRoute
			}
		}
	}
	if bestChoice == nil {
		// No more barrels to collect.
		return nil
	}
	r := &route{
		first: *bestChoice,
		next:  bestFollowingRoute,
		cost:  minCost,
	}
	bestRouteCache[cacheKey] = r
	return r
}

func moveCost(from, to *botPosition) int {
	// Placeholder; expect to refine this a lot.
	return abs(from.coords.x-to.coords.x) + abs(from.coords.y-to.coords.y)
}

func dropCost() int {
	// Fixed cost for dropping off operation.  Will need to refine this.
	return 10
}

func abs(value int) int {
	if value < 0 {
		return -value
	}
	return value
}

func TestBarrels() {
	colours[RED] = "R"
	colours[GREEN] = "G"
	barrels := [2][]coords{{
		{x: 10, y: 10},
		{x: 40, y: 8},
		{x: 45, y: 26},
		{x: 70, y: 35},
		{x: 80, y: 20},
	}, {
		{x: 15, y: 25},
		{x: 38, y: 73},
		{x: 23, y: 73},
		{x: 63, y: 45},
		{x: 69, y: 10},
	}}
	initialBotPosition := &botPosition{coords: coords{x: 0, y: 0}}
	dropZone[RED] = &botPosition{coords: coords{x: 20, y: 100}}
	dropZone[GREEN] = &botPosition{coords: coords{x: 80, y: 100}}
	r := bestRoute(initialBotPosition, initialState(barrels))
	fmt.Printf("Best route is %v\n", r)
	fmt.Printf("Route cache:\n")
	for key, route := range bestRouteCache {
		if route.len() >= 9 {
			fmt.Printf("%v -> %v\n", key, route)
		}
	}
}

var colours [2]string

func (r *route) String() string {
	return strconv.Itoa(r.cost) + " " + r.choices()
}

func (r *route) choices() string {
	s := colours[r.first.colour] + r.first.coords.String()
	if r.next != nil {
		s += "-" + r.next.choices()
	}
	return s
}

func (r *route) len() int {
	if r.next != nil {
		return 1 + r.next.len()
	}
	return 1
}
