package main

import "math"

type (
	RGB struct {
		R, G, B float32
	}

	SRGB struct {
		R, G, B float32
	}

	Oklab struct {
		L, A, B float32
	}
)

func sRGB_transfer(f float32) float32 {
	if f >= 0.04045 {
		return float32(math.Pow(float64((f+0.055)/(1+0.055)), 2.4))
	} else {
		return f / 12.92
	}

}

func sRGB_transfer_inv(f float32) float32 {
	if f >= 0.0031308 {
		return float32(1.055*math.Pow(float64(f), 1.0/2.4) - 0.055)
	} else {
		return f * 12.92
	}
}

func (c RGB) ToSRGB() SRGB {
	return SRGB{
		R: sRGB_transfer(c.R),
		G: sRGB_transfer(c.G),
		B: sRGB_transfer(c.B),
	}
}

func (c SRGB) ToRGB() RGB {
	return RGB{
		R: sRGB_transfer_inv(c.R),
		G: sRGB_transfer_inv(c.G),
		B: sRGB_transfer_inv(c.B),
	}
}

func (c SRGB) ToOklab() Oklab {
	l := 0.4121656120*c.R + 0.5362752080*c.G + 0.0514575653*c.B
	m := 0.2118591070*c.R + 0.6807189584*c.G + 0.1074065790*c.B
	s := 0.0883097947*c.R + 0.2818474174*c.G + 0.6302613616*c.B

	l = float32(math.Cbrt(float64(l)))
	m = float32(math.Cbrt(float64(m)))
	s = float32(math.Cbrt(float64(s)))

	return Oklab{
		L: 0.2104542553*l + 0.7936177850*m - 0.0040720468*s,
		A: 1.9779984951*l - 2.4285922050*m + 0.4505937099*s,
		B: 0.0259040371*l + 0.7827717662*m - 0.8086757660*s,
	}
}

func clamp(min, mid, max float32) float32 {
	if mid < min {
		return min
	} else if mid > max {
		return max
	} else {
		return mid
	}
}

func (c Oklab) ToSRGB() SRGB {
	l := c.L + 0.3963377774*c.A + 0.2158037573*c.B
	m := c.L - 0.1055613458*c.A - 0.0638541728*c.B
	s := c.L - 0.0894841775*c.A - 1.2914855480*c.B

	l = l * l * l
	m = m * m * m
	s = s * s * s

	return SRGB{
		R: clamp(0, +4.0767245293*l - 3.3072168827*m + 0.2307590544*s, 1),
		G: clamp(0, -1.2681437731*l + 2.6093323231*m - 0.3411344290*s, 1),
		B: clamp(0, -0.0041119885*l - 0.7034763098*m + 1.7068625689*s, 1),
	}
}

func (c Oklab) ToRGB() RGB {
	return c.ToSRGB().ToRGB()
}

func (c RGB) ToOklab() Oklab {
	return c. ToSRGB().ToOklab()
}

func (c RGB) ToRGBA(alpha byte) RGBA {
	return RGBA {
		R: byte(math.Round(float64(c.R * 255))),
			G: byte(math.Round(float64(c.G * 255))),
			B: byte(math.Round(float64(c.B * 255))),
		A: alpha,
	}
}

func (c RGBA) ToRGB() RGB {
	return RGB {
		R: float32(c.R) / 255,
			G: float32(c.G) / 255,
			B: float32(c.B) / 255,
		}
}

func (a Oklab) Lerp(b Oklab, r float32) Oklab {
	ir := 1 - r
	return Oklab{
		L: float32(a.L)*ir + float32(b.L)*r,
		A: float32(a.A)*ir + float32(b.A)*r,
		B: float32(a.B)*ir + float32(b.B)*r,
	}
}
