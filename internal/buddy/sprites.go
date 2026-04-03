package buddy

// sprites maps species to animation frames (each frame is a slice of lines).
var sprites = map[string][][]string{
	"duck": {
		{
			`    __      `,
			`  <(o )___  `,
			`   (  ._>   `,
			`    ` + "`" + `--'    `,
		},
		{
			`    __      `,
			`  <(o )___  `,
			`   (  ._>   `,
			`    ` + "`" + `--'~   `,
		},
		{
			`    __      `,
			`  <(o )___  `,
			`   (  .__>  `,
			`    ` + "`" + `--'    `,
		},
	},
	"cat": {
		{
			`   /\_/\    `,
			`  ( o   o)  `,
			`  (  w  )   `,
			`  (")_(")   `,
		},
		{
			`   /\_/\    `,
			`  ( o   o)  `,
			`  (  w  )   `,
			`  (")_(")~  `,
		},
	},
	"ghost": {
		{
			`   .----.   `,
			`  / o  o \  `,
			`  |      |  `,
			"  ~`~``~`~  ",
		},
		{
			`   .----.   `,
			`  / o  o \  `,
			`  |      |  `,
			"  `~`~~`~`  ",
		},
	},
	"robot": {
		{
			`   .[||].   `,
			`  [ o  o ]  `,
			`  [ ==== ]  `,
			"  `------'  ",
		},
		{
			`   .[||].   `,
			`  [ o  o ]  `,
			`  [ -==- ]  `,
			"  `------'  ",
		},
	},
	"owl": {
		{
			`   /\  /\   `,
			`  ((o)(o))  `,
			`  (  ><  )  `,
			"   `----'   ",
		},
		{
			`   /\  /\   `,
			`  ((o)(o))  `,
			`  (  ><  )  `,
			`   .----.   `,
		},
	},
	"blob": {
		{
			`   .----.   `,
			`  ( o  o )  `,
			`  (      )  `,
			"   `----'   ",
		},
		{
			`  .------.  `,
			` (  o  o  ) `,
			` (        ) `,
			"  `------'  ",
		},
	},
	"dragon": {
		{
			`  /^\  /^\  `,
			` <  o  o  > `,
			` (   ~~   ) `,
			"  `-vvvv-'  ",
		},
		{
			`  /^\  /^\  `,
			` <  o  o  > `,
			` (        ) `,
			"  `-vvvv-'  ",
		},
	},
	"penguin": {
		{
			`  .---.     `,
			`  (o>o)     `,
			` /(   )\    `,
			"  `---'     ",
		},
		{
			`  .---.     `,
			`  (o>o)     `,
			` |(   )|    `,
			"  `---'     ",
		},
	},
	"snail": {
		{
			` o    .--.  `,
			`  \  ( @ )  `,
			`   \_` + "`" + `--'   `,
			`  ~~~~~~~   `,
		},
		{
			`  o   .--.  `,
			`  |  ( @ )  `,
			`   \_` + "`" + `--'   `,
			`  ~~~~~~~   `,
		},
	},
	"mushroom": {
		{
			` .-o-OO-o-. `,
			`(__________)`,
			`   |o  o|   `,
			`   |____|   `,
		},
		{
			` .-O-oo-O-. `,
			`(__________)`,
			`   |o  o|   `,
			`   |____|   `,
		},
	},
	"octopus": {
		{
			`   .----.   `,
			`  ( o  o )  `,
			`  (______)  `,
			`  /\/\/\/\  `,
		},
		{
			`   .----.   `,
			`  ( o  o )  `,
			`  (______)  `,
			`  \/\/\/\/  `,
		},
	},
	"cactus": {
		{
			` n  ____  n `,
			` | |o  o| | `,
			` |_|    |_| `,
			`   |    |   `,
		},
		{
			`    ____    `,
			` n |o  o| n `,
			` |_|    |_| `,
			`   |    |   `,
		},
	},
}

// RenderSprite returns the ASCII art lines for a companion at the given
// animation frame. Frame wraps around the available frames for the species.
func RenderSprite(c *Companion, frame int) []string {
	frames, ok := sprites[c.Species]
	if !ok || len(frames) == 0 {
		return []string{"  (?.?)  ", " unknown "}
	}
	idx := frame % len(frames)
	// Return a copy so callers can't mutate the sprite data.
	result := make([]string, len(frames[idx]))
	copy(result, frames[idx])
	return result
}
