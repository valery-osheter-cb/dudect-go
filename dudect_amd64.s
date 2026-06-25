//go:build amd64 && !purego

#include "textflag.h"

TEXT ·cpuTicks(SB), NOSPLIT, $0-8
	LFENCE
	RDTSC
	SHLQ $32, DX
	ORQ  DX, AX
	MOVQ AX, ret+0(FP)
	RET
