struct dtr_reg {
  unsigned short limit __attribute__((packed));
  unsigned long  base  __attribute__((packed));
};

// Borrowed from `static void go32_sgdt (const char *arg, int from_tty)` in go32-nat.c
void SgdtAddr(unsigned short* limit ,unsigned long* base) {
  struct dtr_reg gdtr;
  __asm__ __volatile__ ("sgdt   %0" : "=m" (gdtr) : /* no inputs */ );
  *limit = gdtr.limit;
  *base = gdtr.base;
  return;
}
