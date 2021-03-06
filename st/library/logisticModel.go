package library

import (
	"errors"
	"github.com/nytlabs/gojee"                 // jee
	"github.com/nytlabs/streamtools/st/blocks" // blocks
	"github.com/nytlabs/streamtools/st/util"   // util
	"math"
	"math/rand"
)

// specify those channels we're going to use to communicate with streamtools
type LogisticModel struct {
	blocks.Block
	queryrule chan chan interface{}
	inrule    chan interface{}
	in        chan interface{}
	out       chan interface{}
	quit      chan interface{}
}

// we need to build a simple factory so that streamtools can make new blocks of this kind
func NewLogisticModel() blocks.BlockInterface {
	return &LogisticModel{}
}

// Setup is called once before running the block. We build up the channels and specify what kind of block this is.
func (b *LogisticModel) Setup() {
	b.Kind = "LogisticModel"
	b.Desc = "returns 1 or 0 depending on the model parameters and feature values"
	b.inrule = b.InRoute("rule")
	b.queryrule = b.QueryRoute("rule")
	b.in = b.InRoute("in")
	b.quit = b.Quit()
	b.out = b.Broadcast()
}
func logit(x float64) float64 {
	return 1 / (1 + math.Exp(-x))
}

// Run is the block's main loop. Here we listen on the different channels we set up.
func (b *LogisticModel) Run() {

	var β []float64
	var featurePaths []string
	var featureTrees []*jee.TokenTree
	var err error

	for {
	Loop:
		select {
		case rule := <-b.inrule:
			β, err = util.ParseArrayFloat(rule, "Weights")
			if err != nil {
				b.Error(err)
				continue
			}
			featurePaths, err = util.ParseArrayString(rule, "FeaturePaths")
			if err != nil {
				b.Error(err)
				continue
			}
			featureTrees = make([]*jee.TokenTree, len(featurePaths))
			for i, path := range featurePaths {
				token, err := jee.Lexer(path)
				if err != nil {
					b.Error(err)
					break
				}
				tree, err := jee.Parser(token)
				if err != nil {
					b.Error(err)
					break
				}
				featureTrees[i] = tree
			}
		case <-b.quit:
			// quit the block
			return
		case msg := <-b.in:
			if featureTrees == nil {
				continue
			}
			x := make([]float64, len(featureTrees))
			for i, tree := range featureTrees {
				feature, err := jee.Eval(tree, msg)
				if err != nil {
					b.Error(err)
					break Loop
				}
				fi, ok := feature.(float64)
				if !ok {
					b.Error(errors.New("features must be float64"))
					break Loop
				}
				x[i] = fi
			}
			μ := 0.0
			for i, βi := range β {
				μ += βi * x[i]
			}
			var y float64
			if rand.Float64() <= logit(μ) {
				y = 1
			} else {
				y = 0
			}
			b.out <- map[string]interface{}{
				"Response": y,
			}

		case respChan := <-b.queryrule:
			// deal with a query request
			out := map[string]interface{}{
				"Weights":      β,
				"FeaturePaths": featurePaths,
			}
			respChan <- out
		}
	}
}
