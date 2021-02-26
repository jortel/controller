package itinerary

import (
	"github.com/onsi/gomega"
	"testing"
)

var (
	p1 Flag = 0x01
	p2 Flag = 0x02
	p3 Flag = 0x04
)

type TestPredicate struct {
}

func (p *TestPredicate) Evaluate(f Flag) (bool, error) {
	switch f {
	case p1:
		return false, nil
	case p2:
		return true, nil
	case p3:
		return true, nil
	}

	return true, nil
}

func TestExport(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		{
			Name:        "ONE",
			Description: "The one.",
			Children: []Step{
				{Name: "A"},
				{Name: "B", All: p1},
				{Name: "C"},
			},
		},
		{
			Name: "TWO",
			Children: []Step{
				{Name: "D"},
				{Name: "E"},
				{
					Name: "F",
					Children: []Step{
						{Name: "fa"},
						{Name: "fb"},
						{Name: "fc", All: p1},
					},
				},
				{Name: "G"},
			},
		},
		{Name: "THREE"},
	}

	pred := &TestPredicate{}
	pipeline, err := itinerary.Pipeline(pred)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(len(pipeline.Tasks)).To(gomega.Equal(len(itinerary)))

	// Add parallel pipeline to step C.
	stepC, _ := pipeline.Get("ONE/C")
	stepC.Children = []*Task{
		{Name: "p1", Parallel: true},
		{Name: "p2", Parallel: true},
		{Name: "p3", Parallel: true},
	}
	// Parent.
	g.Expect(pipeline.Tasks[0].Parent()).To(gomega.Equal(""))
	g.Expect(stepC.Parent()).To(gomega.Equal("ONE"))
	// Run.
	phase := "ONE"
	current, found := pipeline.Get(phase)
	g.Expect(found).To(gomega.BeTrue())
	g.Expect(current).ToNot(gomega.BeNil())
	for {
		task := pipeline.Current()
		next, done, err := pipeline.Next()
		if done || err != nil {
			break
		}
		g.Expect(task).ToNot(gomega.BeNil())
		g.Expect(next).ToNot(gomega.BeNil())
	}
}

/*
func TestGet(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "TWO"},
			Step{Name: "THREE"},
		},
	}

	current, err := itinerary.Get("TWO")
	g.Expect(err).To(gomega.BeNil())
	g.Expect(current.Name).To(gomega.Equal("TWO"))
}

func TestNext(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "TWO"},
			Step{Name: "THREE"},
		},
	}

	// ONE
	next, done, err := itinerary.Next("ONE")
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeFalse())
	g.Expect(next.Name).To(gomega.Equal("TWO"))
	// TWO
	next, done, err = itinerary.Next(next.Name)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeFalse())
	g.Expect(next.Name).To(gomega.Equal("THREE"))
	// THREE
	next, done, err = itinerary.Next(next.Name)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeTrue())
}

func TestNextWithPredicate(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "ONE-1", All: p1},
			Step{Name: "TWO", All: p2 | p3},
			Step{Name: "THREE", Any: p1 | p2},
		},
	}

	itinerary.Predicate = &TestPredicate{}

	// ONE
	next, done, err := itinerary.Next("ONE")
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeFalse())
	g.Expect(next.Name).To(gomega.Equal("TWO"))
	// TWO
	next, done, err = itinerary.Next(next.Name)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeFalse())
	g.Expect(next.Name).To(gomega.Equal("THREE"))
	// THREE
	next, done, err = itinerary.Next(next.Name)
	g.Expect(err).To(gomega.BeNil())
	g.Expect(done).To(gomega.BeTrue())

	// Step Not Found
	next, done, err = itinerary.Next("unknown")
	g.Expect(errors.Is(err, StepNotFound)).To(gomega.BeTrue())

}

func TestFirst(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "ONE-1", All: p1},
			Step{Name: "TWO", All: p2 | p3},
			Step{Name: "THREE", Any: p1 | p2},
		},
	}

	itinerary.Predicate = &TestPredicate{}

	// First
	step, err := itinerary.First()
	g.Expect(err).To(gomega.BeNil())
	g.Expect(step.Name).To(gomega.Equal("ONE"))
}

func TestList(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "ONE-1", All: p1},
			Step{Name: "TWO", All: p2 | p3},
			Step{Name: "THREE", Any: p1 | p2},
		},
	}

	itinerary.Predicate = &TestPredicate{}

	list, err := itinerary.List()
	g.Expect(err).To(gomega.BeNil())
	g.Expect(len(list)).To(gomega.Equal(3))
	g.Expect(list[0].Name).To(gomega.Equal("ONE"))
	g.Expect(list[1].Name).To(gomega.Equal("TWO"))
	g.Expect(list[2].Name).To(gomega.Equal("THREE"))
}

func TestProgress(t *testing.T) {
	g := gomega.NewGomegaWithT(t)

	itinerary := Itinerary{
		Name: "Test",
		Pipeline: Pipeline{
			Step{Name: "ONE"},
			Step{Name: "ONE-1", All: p1},
			Step{Name: "TWO", All: p2 | p3},
			Step{Name: "THREE", Any: p1 | p2},
		},
	}

	itinerary.Predicate = &TestPredicate{}

	// First
	list, err := itinerary.List()
	g.Expect(err).To(gomega.BeNil())
	for i, step := range list {
		report, err := itinerary.Progress(step.Name)
		g.Expect(err).To(gomega.BeNil())
		g.Expect(report.Total).To(gomega.Equal(int64(len(list))))
		g.Expect(report.Completed).To(gomega.Equal(int64(i)))
	}
}
*/
