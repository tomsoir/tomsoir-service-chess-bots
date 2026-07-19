package roster

import (
	"fmt"
	"hash/fnv"
	"math/rand/v2"
)

// Identity is a stable fake player used in the lobby fleet.
type Identity struct {
	ID          string
	Name        string
	CountryCode string
	Score       int
	EngineLevel int
	EmojiRate   float64 // 0..1 likelihood to send emoji on triggers
}

// Score band mapping aligned with remapped Play AI levels.
var levelScoreBands = map[int][2]int{
	1: {100, 250},
	2: {400, 650},
	3: {750, 1050},
	4: {1250, 1550},
	5: {1800, 2200},
}

// localeNames pairs ISO country codes with first names common there.
var localeNames = []struct {
	Country string
	Names   []string
}{
	{"US", []string{"James", "Emily", "Michael", "Ashley", "Chris", "Megan", "Ryan", "Lauren", "Justin", "Hannah"}},
	{"GB", []string{"Oliver", "Amelia", "Harry", "Chloe", "Jack", "Sophie", "George", "Emily", "Charlie", "Isla"}},
	{"DE", []string{"Lukas", "Mia", "Leon", "Emma", "Paul", "Hannah", "Finn", "Marie", "Jonas", "Lina"}},
	{"FR", []string{"Lucas", "Léa", "Hugo", "Chloé", "Louis", "Manon", "Gabriel", "Camille", "Arthur", "Inès"}},
	{"ES", []string{"Pablo", "Lucía", "Diego", "María", "Álvaro", "Sofía", "Carlos", "Carmen", "Javier", "Elena"}},
	{"IT", []string{"Lorenzo", "Giulia", "Matteo", "Sofia", "Alessandro", "Aurora", "Leonardo", "Alice", "Francesco", "Giorgia"}},
	{"BR", []string{"Gabriel", "Ana", "Pedro", "Beatriz", "Lucas", "Julia", "Rafael", "Larissa", "Mateus", "Fernanda"}},
	{"MX", []string{"Santiago", "Valentina", "Mateo", "Camila", "Sebastián", "Ximena", "Diego", "Renata", "Emiliano", "Sofía"}},
	{"CA", []string{"Liam", "Olivia", "Noah", "Emma", "Ethan", "Ava", "Lucas", "Sophia", "Mason", "Charlotte"}},
	{"AU", []string{"Jack", "Charlotte", "William", "Olivia", "Thomas", "Mia", "James", "Amelia", "Henry", "Isla"}},
	{"JP", []string{"Haruto", "Yui", "Sota", "Aoi", "Yuto", "Hina", "Ren", "Sakura", "Hiroto", "Mei"}},
	{"KR", []string{"Minjun", "Seoyeon", "Jiwon", "Hayun", "Dooyun", "Suji", "Hyunwoo", "Jimin", "Seojun", "Yuna"}},
	{"IN", []string{"Aarav", "Aisha", "Vihaan", "Ananya", "Arjun", "Diya", "Rohan", "Isha", "Kabir", "Meera"}},
	{"NL", []string{"Daan", "Emma", "Sem", "Julia", "Luuk", "Sophie", "Finn", "Sara", "Bram", "Lotte"}},
	{"SE", []string{"Erik", "Alice", "Lars", "Ella", "Oskar", "Maja", "Johan", "Wilma", "Nils", "Alva"}},
	{"PL", []string{"Jakub", "Zuzanna", "Antoni", "Julia", "Jan", "Maja", "Szymon", "Lena", "Franciszek", "Oliwia"}},
	{"AR", []string{"Tomás", "Valentina", "Benjamín", "Emma", "Santiago", "Catalina", "Mateo", "Martina", "Joaquín", "Isabella"}},
	{"PT", []string{"João", "Maria", "Rodrigo", "Ana", "Tiago", "Beatriz", "Miguel", "Inês", "Francisco", "Leonor"}},
	{"IE", []string{"Conor", "Aoife", "Sean", "Ciara", "Cian", "Saoirse", "Oisin", "Niamh", "Patrick", "Orla"}},
	{"NZ", []string{"Oliver", "Charlotte", "Jack", "Olivia", "Noah", "Isla", "Leo", "Amelia", "Hunter", "Mia"}},
	{"TR", []string{"Emir", "Zeynep", "Yusuf", "Elif", "Eymen", "Asya", "Mert", "Defne", "Kerem", "Ecrin"}},
	{"UA", []string{"Andriy", "Olena", "Dmytro", "Sofia", "Oleksandr", "Anna", "Maksym", "Maria", "Ivan", "Yulia"}},
	{"CZ", []string{"Jakub", "Eliška", "Jan", "Tereza", "Tomáš", "Adéla", "Matyáš", "Viktorie", "Adam", "Natálie"}},
	{"RO", []string{"Andrei", "Maria", "Alexandru", "Elena", "David", "Ioana", "Stefan", "Andreea", "Mihai", "Ana"}},
}

const rosterSize = 60

// DefaultRoster builds a fixed pool of bot identities with stable UUIDs,
// realistic first names, and matching country codes.
func DefaultRoster() []Identity {
	out := make([]Identity, 0, rosterSize)
	usedNames := map[string]int{}
	for i := 0; i < rosterSize; i++ {
		locale := localeNames[stableInt(i, "locale")%len(localeNames)]
		namePool := locale.Names
		base := namePool[stableInt(i, "name")%len(namePool)]
		name := uniquifyName(base, usedNames, i)
		usedNames[base]++

		level := (i % 5) + 1
		band := levelScoreBands[level]
		score := band[0] + stableInt(i, "score")%(band[1]-band[0]+1)
		out = append(out, Identity{
			ID:          stableUUID(i),
			Name:        name,
			CountryCode: locale.Country,
			Score:       score,
			EngineLevel: level,
			EmojiRate:   0.12 + float64(stableInt(i, "emoji")%35)/100,
		})
	}
	return out
}

func uniquifyName(base string, used map[string]int, i int) string {
	n := used[base]
	if n == 0 {
		return base
	}
	// Occasional last-initial style when first names collide (looks human).
	initials := "ABCDEFGHJKLMNPRSTW"
	ch := initials[stableInt(i, "initial")%len(initials)]
	return fmt.Sprintf("%s %c.", base, ch)
}

// LevelForScore picks an engine level whose score band is closest to target.
func LevelForScore(score int) int {
	best := 3
	bestDist := 1 << 30
	for level, band := range levelScoreBands {
		mid := (band[0] + band[1]) / 2
		d := score - mid
		if d < 0 {
			d = -d
		}
		if d < bestDist {
			bestDist = d
			best = level
		}
	}
	return best
}

// ScoreNear returns a score within ±200 of target, clamped to the level band when possible.
func ScoreNear(target, level int) int {
	band := levelScoreBands[level]
	lo := target - 200
	hi := target + 200
	if lo < band[0] {
		lo = band[0]
	}
	if hi > band[1] {
		hi = band[1]
	}
	if lo > hi {
		return (band[0] + band[1]) / 2
	}
	if lo == hi {
		return lo
	}
	return lo + rand.IntN(hi-lo+1)
}

func WithinBand(a, b int) bool {
	d := a - b
	if d < 0 {
		d = -d
	}
	return d <= 200
}

func stableUUID(i int) string {
	h := fnv.New128a()
	_, _ = h.Write([]byte("tomsoir-chess-bot-v2"))
	_, _ = h.Write([]byte{byte(i), byte(i >> 8)})
	sum := h.Sum(nil)
	sum[6] = (sum[6] & 0x0f) | 0x40
	sum[8] = (sum[8] & 0x3f) | 0x80
	return fmt.Sprintf("%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		sum[0], sum[1], sum[2], sum[3], sum[4], sum[5], sum[6], sum[7],
		sum[8], sum[9], sum[10], sum[11], sum[12], sum[13], sum[14], sum[15])
}

func stableInt(i int, salt string) int {
	h := fnv.New32a()
	_, _ = h.Write([]byte(salt))
	_, _ = h.Write([]byte{byte(i)})
	return int(h.Sum32())
}
