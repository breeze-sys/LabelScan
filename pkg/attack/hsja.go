package attack

import (
	"context"

	"Label-Only-MIA-Go/pkg/core"
	"Label-Only-MIA-Go/pkg/mathutils"
	"math"
)

type HSJAConfig struct {
	MaxQueries    int
	MaxIterations int
	NumEvals      int
	InitEvals     int
	ClipMin       float32
	ClipMax       float32
}

type HSJA struct {
	config HSJAConfig
}

func NewHSJA(cfg HSJAConfig) *HSJA {
	if cfg.MaxQueries == 0 {
		cfg.MaxQueries = 10000
	}
	if cfg.NumEvals == 0 {
		cfg.NumEvals = 100
	}
	if cfg.MaxIterations == 0 {
		cfg.MaxIterations = 50
	}
	if cfg.InitEvals == 0 {
		cfg.InitEvals = 100
	}
	if cfg.ClipMax == 0 {
		cfg.ClipMax = 1.0
	}
	return &HSJA{config: cfg}
}

// Attack estimates the distance from a sample to the target model decision boundary.
func (atk *HSJA) Attack(sample core.Sample, model core.Model) core.AttackResult {
	return atk.AttackContext(context.Background(), sample, model)
}

func (atk *HSJA) AttackContext(ctx context.Context, sample core.Sample, model core.Model) core.AttackResult {
	queries := 0
	original := sample.Data
	targetLabel := sample.Label

	predictFunc := func(img []float32) int {
		if ctx.Err() != nil {
			return targetLabel
		}
		queries++
		l, err := predictContext(ctx, model, core.Image(img))
		if err != nil {
			return targetLabel
		}
		return l
	}

	xAdv := atk.initialize(ctx, original, targetLabel, predictFunc)
	if xAdv == nil || ctx.Err() != nil {
		return core.AttackResult{
			SampleID: sample.ID,
			Distance: 0.0,
			Queries:  queries,
		}
	}

	xAdv = atk.binarySearch(ctx, original, xAdv, targetLabel, predictFunc)
	dist := mathutils.L2Distance(original, xAdv)

	for i := 0; i < atk.config.MaxIterations; i++ {
		if ctx.Err() != nil || queries >= atk.config.MaxQueries {
			break
		}

		delta := atk.computeDelta(float32(dist), i)
		grad := atk.approximateGradient(ctx, xAdv, targetLabel, delta, model, &queries)

		stepSize := atk.computeStepSize(float32(dist), i)
		stepVec := mathutils.VectorScale(grad, stepSize)
		xNew := mathutils.VectorAdd(xAdv, stepVec)

		xNew = mathutils.Clip(xNew, atk.config.ClipMin, atk.config.ClipMax)

		xNew = atk.binarySearch(ctx, original, xNew, targetLabel, predictFunc)
		if ctx.Err() != nil {
			break
		}

		newDist := mathutils.L2Distance(original, xNew)
		if newDist < dist {
			dist = newDist
			xAdv = xNew
		}
	}

	return core.AttackResult{
		SampleID: sample.ID,
		Distance: dist,
		Queries:  queries,
	}
}

func predictContext(ctx context.Context, model core.Model, img core.Image) (int, error) {
	if m, ok := model.(core.ContextModel); ok {
		return m.PredictContext(ctx, img)
	}
	if ctx.Err() != nil {
		return -1, ctx.Err()
	}
	return model.Predict(img)
}

func predictBatchContext(ctx context.Context, model core.Model, imgs []core.Image) ([]int, error) {
	if m, ok := model.(core.ContextModel); ok {
		return m.PredictBatchContext(ctx, imgs)
	}
	if ctx.Err() != nil {
		return nil, ctx.Err()
	}
	return model.PredictBatch(imgs)
}

// initialize searches for an initial sample that crosses the target boundary.
func (atk *HSJA) initialize(ctx context.Context, original []float32, label int, predict func([]float32) int) []float32 {
	if predict(original) != label {
		return original
	}
	inputSize := len(original)
	for i := 0; i < atk.config.InitEvals; i++ {
		if ctx.Err() != nil {
			return nil
		}
		noise := mathutils.GenUniform(inputSize, float64(atk.config.ClipMin), float64(atk.config.ClipMax))
		if predict(noise) != label {
			return noise
		}
	}
	return nil
}

// binarySearch refines an adversarial sample onto the decision boundary.
func (atk *HSJA) binarySearch(ctx context.Context, original, adversarial []float32, targetLabel int, predict func([]float32) int) []float32 {
	low := 0.0
	high := 1.0
	boundaryPoint := adversarial

	for i := 0; i < 15; i++ {
		if ctx.Err() != nil {
			break
		}
		mid := (low + high) / 2.0
		candidate := mathutils.Interpolate(original, adversarial, float32(mid))
		candidate = mathutils.Clip(candidate, atk.config.ClipMin, atk.config.ClipMax)

		if predict(candidate) != targetLabel {
			high = mid
			boundaryPoint = candidate
		} else {
			low = mid
		}
	}
	return boundaryPoint
}

// approximateGradient estimates the boundary normal with batched label queries.
func (atk *HSJA) approximateGradient(ctx context.Context, sample []float32, label int, delta float32, model core.Model, queries *int) []float32 {
	numEvals := atk.config.NumEvals
	inputSize := len(sample)

	batchImgs := make([]core.Image, numEvals)
	noises := make([][]float32, numEvals)

	for j := 0; j < numEvals; j++ {
		if ctx.Err() != nil {
			return mathutils.NewVector(inputSize, 0)
		}
		noise := mathutils.GenGaussian(inputSize, 0, 1)
		noise = mathutils.Normalize(noise)
		noises[j] = noise

		perturbation := mathutils.VectorScale(noise, delta)
		posPoint := mathutils.VectorAdd(sample, perturbation)
		posPoint = mathutils.Clip(posPoint, atk.config.ClipMin, atk.config.ClipMax)

		batchImgs[j] = core.Image(posPoint)
	}

	preds, err := predictBatchContext(ctx, model, batchImgs)
	if err != nil {
		return mathutils.NewVector(inputSize, 0)
	}
	*queries += numEvals

	var validDirections [][]float32
	for j, pred := range preds {
		if pred != label {
			validDirections = append(validDirections, noises[j])
		} else {
			validDirections = append(validDirections, mathutils.VectorScale(noises[j], -1.0))
		}
	}

	if len(validDirections) == 0 {
		return mathutils.NewVector(inputSize, 0)
	}
	grad := mathutils.MeanVector(validDirections)
	return mathutils.Normalize(grad)
}

func (atk *HSJA) computeDelta(dist float32, iter int) float32 {
	if iter == 0 {
		return 0.1
	}
	return dist * 0.1 / float32(math.Sqrt(float64(iter)))
}

func (atk *HSJA) computeStepSize(dist float32, iter int) float32 {
	return dist / float32(math.Sqrt(float64(iter)+1))
}
