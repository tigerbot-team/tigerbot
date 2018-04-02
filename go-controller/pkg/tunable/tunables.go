package tunable

import (
	"fmt"
	"sync/atomic"
)

type Tunable struct {
	Name  string
	Value int64
}

func (t *Tunable) Add(delta int) {
	newV := atomic.AddInt64(&t.Value, int64(delta))
	fmt.Println("Tunable", t.Name, "=", newV)
}

func (t *Tunable) Get() int {
	return int(atomic.LoadInt64(&t.Value))
}

type Tunables struct {
	All      []*Tunable
	selected int
}

func (t *Tunables) Create(name string, value int) *Tunable {
	newTunable := &Tunable{
		Name:  name,
		Value: int64(value),
	}
	t.All = append(t.All, newTunable)
	return newTunable
}

func (t *Tunables) SelectNext() {
	t.selected++
	if t.selected >= len(t.All) {
		t.selected = 0
	}
	fmt.Println("Tunable", t.Current().Name, "selected, value:", t.Current().Get())
}

func (t *Tunables) SelectPrev() {
	t.selected--
	if t.selected < 0 {
		t.selected = len(t.All) - 1
	}
	fmt.Println("Tunable", t.Current().Name, "selected, value:", t.Current().Get())
}

func (t *Tunables) Current() *Tunable {
	return t.All[t.selected]
}
