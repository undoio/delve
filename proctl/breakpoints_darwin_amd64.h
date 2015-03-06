#include <sys/types.h>
#include <mach/mach.h>
#include <mach/mach_types.h>
#include <errno.h>

uint64_t
get_control_register(thread_act_t);

kern_return_t
set_control_register(thread_act_t, uint64_t);

kern_return_t
set_debug_register(thread_act_t, int, uint64_t);
