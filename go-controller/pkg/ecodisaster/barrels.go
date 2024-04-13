package ecodisaster

import (
	"fmt"
	"math"
	"math/rand"
	"strconv"

	"github.com/tigerbot-team/tigerbot/go-controller/pkg/challengemode"
)

const (
	// Indices into various arrays.
	RED   = 0
	GREEN = 1

	// Number of barrels that the bot can carry at once.  We'll
	// make sure that these are always all the same colour.
	BOT_CAPACITY = 3

	dxTotal      = float64(2200)
	dyTotal      = float64(2200)
	xBarrelAreaL = float64(300)
	xBarrelAreaR = float64(1900)
	yBarrelAreaB = float64(300)
	yBarrelAreaT = float64(1900)
	xGreenDropL  = float64(400)
	xGreenDropR  = float64(1000)
	xRedDropL    = float64(1200)
	xRedDropR    = float64(1800)
	yDropB       = float64(2000)
	yDropT       = float64(2200)
	xStartL      = float64(1100) - float64(112.5)
	xStartR      = float64(1100) + float64(112.5)
	yStartB      = float64(200)
	yStartT      = float64(600)
)

type coords struct {
	x, y float64
}

func (c coords) String() string {
	return fmt.Sprintf("%.0f:%.0f", c.x, c.y)
}

type arena struct {
	// Coordinates of barrels that still need to be collected.
	barrels [2][]coords

	// Number of barrels that the bot is already carrying.
	botLoad int

	// Colours of the barrels that the bot is already carrying;
	// only significant when botLoad > 0.
	botColours [BOT_CAPACITY]int
}

// The initial state, given specified barrel positions.
func initialState(barrels [2][]coords) *arena {
	return &arena{
		barrels: barrels,
		botLoad: 0,
	}
}

// The following state, if we decide to pick up the Ith barrel of
// colour C.
func nextStateAfterPickUp(state *arena, c int, i int) *arena {
	next := &arena{
		barrels: [2][]coords{nil, nil},
	}
	for i := range next.botColours {
		next.botColours[i] = state.botColours[i]
	}

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

	// Note that the bot is carrying the new barrel.
	next.botColours[state.botLoad] = c
	next.botLoad = state.botLoad + 1

	return next
}

// The following state, if we decide to drop off barrels with colour
// C.
func nextStateAfterDropOff(state *arena, c int) *arena {
	next := &arena{
		barrels: [2][]coords{nil, nil},
	}

	// Update what the bot is carrying.
	for i := range next.botColours {
		next.botColours[i] = state.botColours[i]
	}
	next.botLoad = state.botLoad
	for i := state.botLoad - 1; i >= 0; i-- {
		if state.botColours[i] == c {
			next.botLoad -= 1
		} else {
			break
		}
	}

	// Copy remaining barrel coordinates.  Here we can reuse the
	// slices, as we are treating them as immutable.
	next.barrels[0] = state.barrels[0]
	next.barrels[1] = state.barrels[1]

	return next
}

type choice struct {
	// Choice to pick up another barrel.
	pickUp bool

	// Choice to drop off barrels.
	dropOff bool

	// The colour of the barrel that we choose to pick up next, or
	// of the selected drop off zone.
	colour int

	// The coordinates of the barrel (if picking up).
	coords coords
}

type route struct {
	// The first choice that this route makes.
	first *choice

	// The route following that first choice.
	next *route

	// The overall cost of this route.
	cost int
}

var bestRouteCache = map[string]*route{}

func routeCacheKey(p *challengemode.Position, state *arena) string {
	k := fmt.Sprintf("%v-%v-%v-%v", p, state.barrels[RED], state.barrels[GREEN], state.botLoad)
	for i := 0; i < state.botLoad; i++ {
		k += fmt.Sprintf("-%v", state.botColours[i])
	}
	return k
}

func toCollectBarrel(state *arena, p *challengemode.Position, coords *coords) (int, *challengemode.Position) {
	// Placeholder implementation: cost proportional to distance.
	cost := math.Hypot(coords.x-p.X, coords.y-p.Y)
	endPosition := &challengemode.Position{
		X:       coords.x,
		Y:       coords.y,
		Heading: p.Heading,
	}
	return int(cost), endPosition
}

func toDropOff(state *arena, p *challengemode.Position, dropColour int) (int, *challengemode.Position) {
	// Placeholder implementation: cost proportional to distance.
	if dropColour == RED {
		return toCollectBarrel(state, p, &coords{
			x: (xRedDropL + xRedDropR) / 2,
			y: yDropB,
		})
	} else {
		return toCollectBarrel(state, p, &coords{
			x: (xGreenDropL + xGreenDropR) / 2,
			y: yDropB,
		})
	}
}

// Given a bot position and a set of barrels that still need
// collecting, calculate and return the best route.
func bestRoute(p *challengemode.Position, state *arena) *route {
	cacheKey := routeCacheKey(p, state)
	cachedRoute := bestRouteCache[cacheKey]
	if cachedRoute != nil {
		return cachedRoute
	}
	minCost := 0
	var bestChoice *choice = nil
	var bestFollowingRoute *route

	rememberChoiceIfBestSoFar := func(choice *choice, choiceCost int, followingBestRoute *route) {
		if bestChoice == nil || choiceCost < minCost {
			minCost = choiceCost
			bestChoice = choice
			bestFollowingRoute = followingBestRoute
		}
	}

	// In general, our choices at any moment are:
	//
	// 1. Pick up one of the red barrels.
	// 2. Pick up one of the green barrels.
	// 3. Drop off carried barrels in the red zone.
	// 4. Drop off carried barrels in the green zone.
	//
	// Some of these will be impossible for the current bot state;
	// for example, if the bot is already carrying 3 barrels, we
	// cannot choose to pick up another one.  We handle that kind
	// of choice elimination below.
	//
	// For the remaining possible choices, we calculate the cost
	// of immediately executing that choice, plus the cost of
	// completing the challenge after making that choice.  Then we
	// decide on whichever choice has the least cost.
	if state.botLoad < BOT_CAPACITY {
		// We have space to pick up another barrel.
		for _, choiceColour := range []int{RED, GREEN} {
			for choiceIndex := range state.barrels[choiceColour] {
				choiceIndex := choiceIndex

				// Compute what the remaining state of the
				// arena would be if we picked up the chosen
				// barrel.
				followingState := nextStateAfterPickUp(state, choiceColour, choiceIndex)

				// Compute the cost of moving to
				// collect that barrel, and the
				// position that the bot would then be
				// in.
				thisChoiceCost, endPosition := toCollectBarrel(state, p, &state.barrels[choiceColour][choiceIndex])

				// Compute what the best route would
				// be after this collection.
				followingBestRoute := bestRoute(endPosition, followingState)

				// followingBestRoute cannot be nil
				// here, because we must at least have
				// to do the final drop off.
				rememberChoiceIfBestSoFar(
					&choice{
						pickUp: true,
						colour: choiceColour,
						coords: state.barrels[choiceColour][choiceIndex],
					},
					thisChoiceCost+followingBestRoute.cost,
					followingBestRoute)
			}
		}
	}

	if state.botLoad > 0 {
		// Consider a drop-off.
		dropColour := state.botColours[state.botLoad-1]

		// Compute what the remaining state of the arena would
		// be after this drop off.
		followingState := nextStateAfterDropOff(state, dropColour)

		// Compute the cost of doing the drop off, and the
		// position that the bot would then be in.
		thisChoiceCost, endPosition := toDropOff(state, p, dropColour)

		// Compute what the best route would
		// be after this collection.
		followingBestRoute := bestRoute(endPosition, followingState)

		// followingBestRoute can be nil here, if there are no
		// more barrels left.
		if followingBestRoute != nil {
			thisChoiceCost += followingBestRoute.cost
		}

		rememberChoiceIfBestSoFar(
			&choice{
				dropOff: true,
				colour:  dropColour,
			},
			thisChoiceCost,
			followingBestRoute)
	}

	if bestChoice == nil {
		// No more barrels to collect.
		return nil
	}

	r := &route{
		first: bestChoice,
		next:  bestFollowingRoute,
		cost:  minCost,
	}
	bestRouteCache[cacheKey] = r
	return r
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

	// Compute random positions for the barrels.
	barrels := [2][]coords{}
	for _, colour := range []int{RED, GREEN} {
		fmt.Print(colours[colour] + ":")
		barrels[colour] = make([]coords, 6)
		for colourIndex := range barrels[colour] {
			barrels[colour][colourIndex].x = xBarrelAreaL + rand.Float64()*(xBarrelAreaR-xBarrelAreaL)
			barrels[colour][colourIndex].y = yBarrelAreaB + rand.Float64()*(yBarrelAreaT-yBarrelAreaB)
			fmt.Print(" " + barrels[colour][colourIndex].String())
		}
		fmt.Println("")
	}

	initialBotPosition := &challengemode.Position{
		X:       (xStartL + xStartR) / 2,
		Y:       (yStartB + yStartT) / 2,
		Heading: 90,
	}

	r := bestRoute(initialBotPosition, initialState(barrels))
	fmt.Printf("Best route is %v\n", r)
	if false {
		fmt.Printf("Route cache:\n")
		for key, route := range bestRouteCache {
			if route.len() >= 9 {
				fmt.Printf("%v -> %v\n", key, route)
			}
		}
	}
}

var colours [2]string

func (r *route) String() string {
	return strconv.Itoa(r.cost) + " " + r.choices()
}

func (r *route) choices() string {
	var s string
	if r.first.pickUp {
		s = colours[r.first.colour] + r.first.coords.String()
	} else if r.first.dropOff {
		s = "D" + colours[r.first.colour]
	}
	if r.next != nil {
		s += " " + r.next.choices()
	}
	return s
}

func (r *route) len() int {
	if r.next != nil {
		return 1 + r.next.len()
	}
	return 1
}
