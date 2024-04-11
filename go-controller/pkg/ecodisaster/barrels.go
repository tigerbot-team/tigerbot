package ecodisaster

import "fmt"

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
func nextState(state *world, c int, i int) (*world, bool) {
	next := &world{
		barrels:   [2][]coords{nil, nil},
		botColour: state.botColour,
	}
	dropOffNeeded := false

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
		dropOffNeeded = false
		next.botLoad = 1
		next.botColour = c
	} else if c != state.botColour {
		// Change of colour.
		dropOffNeeded = true
		next.botLoad = 1
		next.botColour = c
	} else if state.botLoad == BOT_CAPACITY {
		// Bot was already full, even though the colour is not
		// changing.
		dropOffNeeded = true
		next.botLoad = 1
	} else {
		// Same colour and bot still has capacity.
		next.botLoad = state.botLoad + 1
	}

	return next, dropOffNeeded
}

type choice struct {
	// The colour of the barrel that we choose to pick up next.
	colour int

	// The coordinates of the barrel.
	coords coords
}

type route struct {
	description string

	// The sequence of barrel choices.
	choices []choice

	// The cost of this route.
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
	var bestDesc string
	for _, nextColour := range []int{RED, GREEN} {
		for i := range state.barrels[nextColour] {
			i := i
			newBotPosition := &botPosition{
				coords: state.barrels[nextColour][i],
			}
			followingState, dropOffNeeded := nextState(state, nextColour, i)
			followingBestRoute := bestRoute(newBotPosition, followingState)

			// The bot is currently at P.  If a drop-off
			// is needed it must move:
			// 1. from P to the drop zone for state.botColour
			// 2. from the drop zone to newBotPosition
			//    (the chosen next barrel)
			// 3. then follow the best route from
			//    newBotPosition onwards.
			// If a drop-off is not needed it must move:
			// 1. from P to newBotPosition
			// 2. then follow the best route from
			//    newBotPosition onwards.
			var thisChoiceCost int
			desc := fmt.Sprintf("%v", state.botLoad)
			if state.botLoad != 0 {
				desc += colours[state.botColour]
			}
			if dropOffNeeded {
				thisChoiceCost = moveCost(p, dropZone[state.botColour]) +
					dropCost() +
					moveCost(dropZone[state.botColour], newBotPosition)
				desc += "-Z" + colours[state.botColour]
			} else {
				thisChoiceCost = moveCost(p, newBotPosition)
			}
			desc += "-" + colours[nextColour] + state.barrels[nextColour][i].String()
			thisChoiceCost += followingBestRoute.cost
			if bestChoice == nil || thisChoiceCost < minCost {
				minCost = thisChoiceCost
				bestChoice = &choice{
					colour: nextColour,
					coords: state.barrels[nextColour][i],
				}
				bestFollowingRoute = followingBestRoute
				bestDesc = desc
			}
		}
	}
	if bestChoice == nil {
		// No more barrels to choose from.
		if state.botLoad == 0 {
			return &route{
				description: "DONE",
				choices:     nil,
				cost:        0,
			}
		} else {
			return &route{
				description: "Z" + colours[state.botColour],
				choices:     nil,
				cost:        moveCost(p, dropZone[state.botColour]) + dropCost(),
			}
		}
	}
	r := &route{
		description: bestDesc + "-" + bestFollowingRoute.description,
		choices:     make([]choice, len(bestFollowingRoute.choices)+1),
		cost:        minCost,
	}
	r.choices[0] = *bestChoice
	for j := 1; j < len(r.choices); j++ {
		r.choices[j] = bestFollowingRoute.choices[j-1]
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
		if len(route.choices) >= 9 && len(route.choices) <= 9 {
			fmt.Printf("%v -> %v\n", key, route)
		}
	}
}

var colours [2]string

func (r *route) String() string {
	s := fmt.Sprintf("%v %v ", r.cost, r.description)
	for _, choice := range r.choices {
		//fmt.Println(s)
		//fmt.Printf("%+v\n", choice)
		s += fmt.Sprintf("-%v%v", colours[choice.colour], choice.coords)
	}
	return s
}
