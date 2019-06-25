package config

import (
	"github.com/agoussia/godes"
	rng "github.com/leesper/go_rng"
)


// Napravljene extenzija postojećih implementacija distribucija
// kako bi se moglo preko interfacea dohvaćat vrijednost iz bilo koje distribucije

// Tako da se sada moze u config datoteci samo promjenit ime i parametri za neku od
// distribucija simulacije, drugo ne treba nis dirat

var repetition = false

func NewExpDistr(lambda float64) *ExpDistr {
	return &ExpDistr{ godes.NewExpDistr(repetition), lambda}
}

func NewNormalDistr(mean float64, sigma float64) *NormalDistr {
	return &NormalDistr{ godes.NewNormalDistr(repetition), mean, sigma}
}
func NewUniformDistr(min float64, max float64) *UniformDistr {
	return &UniformDistr{ godes.NewUniformDistr(repetition), min, max}
}

func NewTriangularDistr(a float64, b float64, c float64) *TriangularDistr {
	return &TriangularDistr{ godes.NewTriangularDistr(repetition), a, b, c}
}


func NewGammaDistr(alpha float64, beta float64) *GammaDistr {
	return &GammaDistr{rng.NewGammaGenerator(getSeed()), alpha, beta}
}

func NewPoissonDistr(lambda float64) *PoissonDistr {
	return &PoissonDistr{rng.NewPoissonGenerator(getSeed()), lambda}
}

func NewLogNormalDistr(mean float64, stddev float64) *LogNormalDistr {
	return &LogNormalDistr{rng.NewLognormalGenerator(getSeed()), mean, stddev}
}




var seedCount int64 = 100000
func getSeed() int64 {

	if repetition {
		seedCount++
		return seedCount
	}
	return godes.GetCurComputerTime()
}


type LogNormalDistr struct {
	*rng.LognormalGenerator
	mean float64
	stddev float64
}


type PoissonDistr struct {
	*rng.PoissonGenerator
	lambda float64
}


type GammaDistr struct {
	*rng.GammaGenerator
	alpha float64
	beta float64
}



type ExpDistr struct {
	*godes.ExpDistr
	lambda float64
}

type NormalDistr struct {
	*godes.NormalDistr
	mean	float64
	sigma	float64
}

type UniformDistr struct {
	*godes.UniformDistr
	min		float64
	max		float64
}

type TriangularDistr struct {
	*godes.TriangularDistr
	a	float64
	b 	float64
	c	float64
}

type Distribution interface {
	nextValue() float64
}

func (d *LogNormalDistr) nextValue() float64 {
	return d.Lognormal(d.mean, d.stddev)
}

func (d *GammaDistr) nextValue() float64 {
	return d.Gamma(d.alpha, d.beta)
}

func (d *PoissonDistr) nextValue() float64 {
	return float64(d.Poisson(d.lambda))
}

func (d *ExpDistr) nextValue() float64 {

	if d.lambda == 0 {
		return 0
	}

	return d.Get(d.lambda)
}

func (d *NormalDistr) nextValue() float64 {
	return d.Get(d.mean, d.sigma)
}

func (d *UniformDistr) nextValue() float64 {
	return d.Get(d.min, d.max)
}

func (d *TriangularDistr) nextValue() float64 {
	return d.Get(d.a, d.b, d.c)
}
