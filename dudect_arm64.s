//go:build arm64 && !purego

#include "textflag.h"

// func cpuTicks() uint64
//
// Reads CNTVCT_EL0, the virtual count register, a free-running counter
// available to user space on all ARM64 platforms.
//   ISB                 -> 0xd5033fdf  (ISB SY)
//   MRS X0, CNTVCT_EL0  -> 0xd53be040  (S3_3_c14_c0_2)
TEXT ·cpuTicks(SB), NOSPLIT, $0-8
	WORD $0xd5033fdf
	WORD $0xd53be040
	MOVD R0, ret+0(FP)
	RET
