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
	6: {2300, 2700},
}

// localeNames pairs ISO country codes with funny chess nicknames.
var localeNames = []struct {
	Country string
	Names   []string
}{
	{"US", []string{"shake_my_bishup", "hung_piece_club", "blunderbuss99", "check_yourself", "rook_and_roll", "pawn_star", "oops_all_blunders", "knight_mare", "castle_crashers", "queen_me_asap"}},
	{"GB", []string{"tea_and_tempo", "en_passant_innit", "stiff_upper_lip_zugzwang", "crumpet_mate", "bish_bash_bosh", "cheeky_fork", "keep_calm_and_castle", "blighty_blunder", "scone_and_skewer", "spot_of_check"}},
	{"DE", []string{"schnell_schach", "bauer_no_friends", "zugzwang_und_fertig", "ritter_der_blunder", "kein_remis", "turm_tempo", "doppelbauer_drama", "matt_in_zwei", "fesselung_fever", "e4_oder_bust"}},
	{"FR", []string{"oui_oui_en_passant", "gambit_baguette", "roi_sans_roque", "fou_de_guerre", "pat_isserie", "echec_et_matelote", "tour_sacre", "blunder_du_jour", "tempo_s_il_vous_plait", "dame_fatale"}},
	{"ES", []string{"siesta_then_mate", "jaque_mate_amigo", "alfil_loco", "peon_perdido", "enroque_rapido", "caballo_loco99", "torre_tapa", "gambito_tapas", "blunder_fiesta", "rey_desnudo"}},
	{"IT", []string{"mamma_mia_mate", "pasta_and_pins", "cavallo_pazzo", "arrocco_espresso", "pedone_perduto", "alfiere_al_dente", "scacco_bello", "gambit_gelato", "tempo_tiramisu", "donna_drammatica"}},
	{"BR", []string{"xeque_mate_samba", "peao_da_galera", "cavalo_maluco", "bispo_brabo", "roque_na_laje", "gambito_churrasco", "blunder_brabo", "dama_do_morro", "tempo_tropicália", "rei_sem_guarda"}},
	{"MX", []string{"jaque_con_salsa", "peon_picante", "caballo_caliente", "alfil_asado", "enroque_con_chile", "gambito_guacamole", "blunder_burrito", "dama_del_desierto", "tempo_tacos", "rey_sin_fiesta"}},
	{"CA", []string{"sorry_i_mated", "maple_leaf_blunder", "eh_passant", "rook_hockey", "pawnch_up", "knight_of_the_north", "castle_tims", "queen_of_poutine", "check_eh", "zugzwang_eh"}},
	{"AU", []string{"gday_mate_in_2", "barbie_and_blunder", "roo_rook", "fair_dinkum_fork", "castling_crikey", "pawn_prawn", "bish_from_down_under", "queen_of_oz", "stalemate_mate", "tempo_too_right"}},
	{"JP", []string{"sente_senpai", "koma_chaos", "castling_kun", "pawn_chan", "rook_sama", "bish_desu", "mate_or_bust_san", "gambit_gohan", "tempo_tokyo", "queen_kawaii"}},
	{"KR", []string{"checkmate_oppa", "pawn_jjigae", "rook_ramen", "bish_bapsang", "castle_kimchi", "gambit_gaming", "blunder_busan", "queen_quick", "tempo_seoul", "knight_namsan"}},
	{"IN", []string{"chai_and_checkmate", "gambit_garam", "rook_raja", "pawn_pani_puri", "bish_biryani", "castle_chai", "blunder_bollywood", "queen_of_spices", "tempo_tandoori", "knight_namaste"}},
	{"NL", []string{"stroopwafel_skewer", "oranje_en_passant", "toren_tulpen", "pion_pils", "paard_power", "rokade_fiets", "blunder_dam", "dame_delft", "tempo_terras", "koning_kaas"}},
	{"SE", []string{"fika_then_fork", "ikea_of_blunders", "torn_och_tempo", "bonde_bork", "hast_och_hurra", "rockad_lagom", "schack_matt_hej", "dam_dalahorse", "tempo_stockholm", "kung_kanel"}},
	{"PL", []string{"pierogi_pin", "szach_mat_proszę", "wieża_wawa", "pionek_power", "skoczek_szalony", "roszada_rapid", "blunder_bigos", "hetman_hurra", "tempo_toruń", "król_kiełbasa"}},
	{"AR", []string{"mate_con_mate", "peon_asado", "caballo_pampeano", "alfil_asado99", "enroque_asado", "gambito_asado", "blunder_bife", "dama_del_sur", "tempo_tango", "rey_rioplatense"}},
	{"PT", []string{"xeque_com_pastel", "peao_do_bairro", "cavalo_lisboa", "bispo_bacalhau", "roque_rapidinho", "gambito_galão", "blunder_belem", "dama_do_tejo", "tempo_tram", "rei_sem_castelo"}},
	{"IE", []string{"craic_and_checkmate", "luck_o_the_fork", "rook_of_dublin", "pawnch_drunk", "bish_begorra", "castling_craic", "blunder_blarney", "queen_of_cork", "tempo_tweed", "knight_of_kerry"}},
	{"NZ", []string{"sweet_as_mate", "kiwi_knight", "rook_of_aotearoa", "pawn_pavlova", "bish_from_the_bush", "castling_cuzzy", "blunder_bro", "queen_of_queenstown", "tempo_tiki", "check_yeah_nah"}},
	{"TR", []string{"çay_and_check", "şah_mat_abi", "kale_kebap", "piyon_pide", "at_aheste", "rok_rakı", "blunder_baklava", "vezir_vay", "tempo_taksim", "şah_şiş"}},
	{"RU", []string{"siberian_skewer", "babushka_blunder", "ladya_loves_you", "peshka_power", "kon_chaos", "rokada_rapid", "mat_v_dva", "ferz_forever", "tempo_tundra", "korol_keksa"}},
	{"CZ", []string{"pivo_and_pins", "sach_mat_prosím", "vez_vltava", "pesak_pils", "kun_chaos", "rosada_rapid", "blunder_brno", "dama_dumplings", "tempo_prague", "kral_knedlik"}},
	{"RO", []string{"mamaliga_mate", "sah_mat_bre", "tura_timis", "pion_power", "cal_crazy", "rocada_rapid", "blunder_bucuresti", "dama_de_dunare", "tempo_transilvania", "rege_fara_tura"}},
}

const rosterSize = 60

// DefaultRoster builds a fixed pool of bot identities with stable UUIDs,
// funny chess nicknames, and matching country codes.
func DefaultRoster() []Identity {
	out := make([]Identity, 0, rosterSize)
	usedNames := map[string]int{}
	for i := 0; i < rosterSize; i++ {
		locale := localeNames[stableInt(i, "locale")%len(localeNames)]
		namePool := locale.Names
		base := namePool[stableInt(i, "name")%len(namePool)]
		name := uniquifyName(base, usedNames, i)
		usedNames[base]++

		level := (i % 6) + 1
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
