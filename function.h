#ifndef _FUNCTION_H
#define _FUNCTION_H

#include <stdio.h>

typedef struct function {
  const char *name;
  unsigned start_line;
  unsigned end_line;
} function;

typedef struct functions_array {
  function** data;
  size_t len;
  size_t cap;
} functions_array;


function* fa_at(functions_array* fa, size_t i);

functions_array* get_functions(const char* fname, const char* contents, unsigned long contents_len);

functions_array* fa_new(size_t cap);

void fa_add(functions_array* fa, function* f);

void fa_free(functions_array* fa);


#endif
