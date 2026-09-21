package main

import "math"

// fitK finds the K that minimizes the loss for the given weights, by
// golden-section search (the loss is unimodal in K in practice). Texel's method
// fits K once, on the starting weights, and then keeps it fixed.
func fitK(d *dataset, x []float64) float64 {
	const phi = 0.6180339887498949

	lo, hi := 0.05, 5.0
	a, b := hi-phi*(hi-lo), lo+phi*(hi-lo)
	fa, fb := d.eval(x, kScale(a), nil), d.eval(x, kScale(b), nil)

	for hi-lo > 1e-6 {
		if fa < fb {
			hi, b, fb = b, a, fa
			a = hi - phi*(hi-lo)
			fa = d.eval(x, kScale(a), nil)
		} else {
			lo, a, fa = a, b, fb
			b = lo + phi*(hi-lo)
			fb = d.eval(x, kScale(b), nil)
		}
	}

	return (lo + hi) / 2
}

// adam is the Adam optimizer (Kingma & Ba): each parameter moves by about lr
// per step in the direction its gradient has been consistently pointing,
// regardless of the gradient's magnitude, which suits weights whose gradients
// differ by orders of magnitude (a queen's value versus one PST square).
type adam struct {
	m, v []float64
	t    int
}

func newAdam(n int) *adam {
	return &adam{m: make([]float64, n), v: make([]float64, n)}
}

// step moves x against grad, leaving frozen parameters untouched.
func (a *adam) step(x, grad []float64, frozen []bool, lr float64) {
	const beta1, beta2, eps = 0.9, 0.999, 1e-9

	a.t++
	c1 := 1 - math.Pow(beta1, float64(a.t))
	c2 := 1 - math.Pow(beta2, float64(a.t))

	for j := range x {
		if frozen[j] {
			continue
		}
		a.m[j] = beta1*a.m[j] + (1-beta1)*grad[j]
		a.v[j] = beta2*a.v[j] + (1-beta2)*grad[j]*grad[j]
		x[j] -= lr * (a.m[j] / c1) / (math.Sqrt(a.v[j]/c2) + eps)
	}
}

// options configures optimize.
type options struct {
	epochs int
	lr     float64 // initial learning rate; falls linearly to 5% of it
	// reg is the strength of an L2 pull, reg*(x-anchor), added to the gradient
	// of every unfrozen weight. The evaluation has directions that change
	// nothing (a constant added to all of a king's PST squares cancels between
	// the two kings; a constant added to a piece's PST is the same as removing
	// it from that piece's material), so the loss has no opinion about them and
	// Adam drifts along them; the pull brings them back to the starting values
	// and also keeps weights with little evidence from wandering.
	reg    float64
	anchor []float64

	progressEvery int
	progress      func(epoch int, lr, loss float64) // may be nil
}

// optimize runs full-batch Adam, calling progress after the first epoch and
// then every progressEvery epochs.
func optimize(d *dataset, x []float64, scale float64, frozen []bool, o options) {
	opt := newAdam(len(x))
	grad := make([]float64, len(x))

	for epoch := 1; epoch <= o.epochs; epoch++ {
		rate := o.lr * (1 - 0.95*float64(epoch-1)/float64(max(o.epochs-1, 1)))
		loss := d.eval(x, scale, grad)
		if o.progress != nil && (epoch == 1 || epoch%o.progressEvery == 0) {
			o.progress(epoch, rate, loss)
		}
		if o.reg != 0 {
			for j := range grad {
				grad[j] += o.reg * (x[j] - o.anchor[j])
			}
		}
		opt.step(x, grad, frozen, rate)
	}
}
