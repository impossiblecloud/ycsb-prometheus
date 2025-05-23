// Copyright 2017 The Cockroach Authors.
//
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or
// implied. See the License for the specific language governing
// permissions and limitations under the License. See the AUTHORS file
// for names of contributors.
//
// Author: Arjun Narayan
//
// ZipfGenerator implements the Incrementing Zipfian Random Number Generator from
// [1]: "Quickly Generating Billion-Record Synthetic Databases"
// by Gray, Sundaresan, Englert, Baclawski, and Weinberger, SIGMOD 1994.

package main

import (
	"math"
	"sync"

	"github.com/pkg/errors"
	log "github.com/sirupsen/logrus"
)

// ZipfGenerator is a random number generator that generates draws from a Zipf
// distribution. Unlike rand.Zipf, this generator supports incrementing the
// imax parameter without performing an expensive recomputation of the
// underlying hidden parameters, which is a pattern used in [1] for efficiently
// generating large volumes of Zipf-distributed records for synthetic data.
// Second, rand.Zipf only supports theta <= 1, we suppose all values of theta.
type ZipfGenerator struct {
	// The underlying RNG
	zipfGenMu ZipfGeneratorMu
	// supplied values
	theta float64
	iMin  uint64
	// internally computed values
	alpha, zeta2 float64
}

// ZipfGeneratorMu holds variables which must be globally synced.
type ZipfGeneratorMu struct {
	mu       sync.RWMutex
	iMax     uint64
	iMaxHead uint64
	eta      float64
	zetaN    float64
}

// NewZipfGenerator constructs a new ZipfGenerator with the given parameters.
// It returns an error if the parameters are outside the accepted range.
func NewZipfGenerator(iMin, iMax uint64, theta float64) (*ZipfGenerator, error) {
	if iMin > iMax {
		return nil, errors.Errorf("iMin %d > iMax %d", iMin, iMax)
	}
	if theta < 0.0 || theta == 1.0 {
		return nil, errors.Errorf("0 < theta, and theta != 1")
	}

	z := ZipfGenerator{
		iMin: iMin,
		zipfGenMu: ZipfGeneratorMu{
			iMax: iMax,
		},
		theta: theta,
	}
	z.zipfGenMu.mu.Lock()
	defer z.zipfGenMu.mu.Unlock()

	// Compute hidden parameters
	zeta2, err := computeZetaFromScratch(2, theta)
	if err != nil {
		return nil, errors.Errorf("Could not compute zeta(2,theta): %s", err)
	}
	var zetaN float64
	zetaN, err = computeZetaFromScratch(iMax+1-iMin, theta)
	if err != nil {
		return nil, errors.Errorf("Could not compute zeta(2,%d): %s", iMax, err)
	}
	z.alpha = 1.0 / (1.0 - theta)
	z.zipfGenMu.eta = (1 - math.Pow(2.0/float64(z.zipfGenMu.iMax+1-z.iMin), 1.0-theta)) / (1.0 - zeta2/zetaN)
	z.zipfGenMu.zetaN = zetaN
	z.zeta2 = zeta2
	return &z, nil
}

// computeZetaIncrementally recomputes zeta(iMax, theta), assuming that
// sum = zeta(oldIMax, theta). It returns zeta(iMax, theta), computed incrementally.
func computeZetaIncrementally(oldIMax, iMax uint64, theta float64, sum float64) (float64, error) {
	if iMax < oldIMax {
		return 0, errors.Errorf("Can't increment iMax backwards!")
	}
	for i := oldIMax + 1; i <= iMax; i++ {
		sum += 1.0 / math.Pow(float64(i), theta)
	}
	return sum, nil
}

// The function zeta computes the value
// zeta(n, theta) = (1/1)^theta + (1/2)^theta + (1/3)^theta + ... + (1/n)^theta
func computeZetaFromScratch(n uint64, theta float64) (float64, error) {
	zeta, err := computeZetaIncrementally(0, n, theta, 0.0)
	if err != nil {
		return zeta, errors.Errorf("could not compute zeta: %s", err)
	}
	return zeta, nil
}

// Uint64 draws a new value between iMin and iMax, with probabilities
// according to the Zipf distribution.
func (z *ZipfGenerator) Uint64(u float64) uint64 {
	z.zipfGenMu.mu.RLock()
	uz := u * z.zipfGenMu.zetaN
	var result uint64
	if uz < 1.0 {
		result = z.iMin
	} else if uz < 1.0+math.Pow(0.5, z.theta) {
		result = z.iMin + 1
	} else {
		spread := float64(z.zipfGenMu.iMax + 1 - z.iMin)
		result = z.iMin + uint64(spread*math.Pow(z.zipfGenMu.eta*u-z.zipfGenMu.eta+1.0, z.alpha))
	}

	log.Debugf("Zip Generator: Uint64[%d, %d] -> %d", z.iMin, z.zipfGenMu.iMax, result)

	z.zipfGenMu.mu.RUnlock()
	return result
}

// IncrementIMax increments, iMax, and recompute the internal values that depend
// on it. It throws an error if the recomputation failed.
func (z *ZipfGenerator) IncrementIMax() error {
	z.zipfGenMu.mu.Lock()
	zetaN, err := computeZetaIncrementally(
		z.zipfGenMu.iMax, z.zipfGenMu.iMax+1, z.theta, z.zipfGenMu.zetaN)
	if err != nil {
		z.zipfGenMu.mu.Unlock()
		return errors.Errorf("Could not incrementally compute zeta: %s", err)
	}
	eta := (1 - math.Pow(2.0/float64(z.zipfGenMu.iMax+1-z.iMin), 1.0-z.theta)) / (1.0 - z.zeta2/zetaN)
	z.zipfGenMu.eta = eta
	z.zipfGenMu.zetaN = zetaN
	z.zipfGenMu.iMax++
	z.zipfGenMu.mu.Unlock()
	return nil
}

// IMaxHead returns the current value of IMaxHead, and increments it after.
func (z *ZipfGenerator) IMaxHead() uint64 {
	z.zipfGenMu.mu.Lock()
	if z.zipfGenMu.iMaxHead < z.zipfGenMu.iMax {
		z.zipfGenMu.iMaxHead = z.zipfGenMu.iMax
	}
	iMaxHead := z.zipfGenMu.iMaxHead
	z.zipfGenMu.iMaxHead++
	z.zipfGenMu.mu.Unlock()
	return iMaxHead
}
