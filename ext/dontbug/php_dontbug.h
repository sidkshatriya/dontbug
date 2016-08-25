/*
 * Copyright 2016 Sidharth Kshatriya
 *
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 */

#ifndef PHP_DONTBUG_H
#define PHP_DONTBUG_H

extern zend_module_entry dontbug_module_entry;
#define phpext_dontbug_ptr &dontbug_module_entry

#define PHP_DONTBUG_VERSION "0.0.1"

#if defined(__GNUC__) && __GNUC__ >= 4
#	define PHP_DONTBUG_API __attribute__ ((visibility("default")))
#else
#	define PHP_DONTBUG_API
#endif

#ifdef ZTS
#include "TSRM.h"
#endif

#define DONTBUG_G(v) ZEND_MODULE_GLOBALS_ACCESSOR(dontbug, v)

#if defined(ZTS) && defined(COMPILE_DL_DONTBUG)
ZEND_TSRMLS_CACHE_EXTERN()
#endif

#define PHP_DONTBUG_MAX_PATH_LEN 128

int dontbug_break_location(zend_string* filename, zend_execute_data *execute_data, int lineno);
int dontbug_common_user_opcode_handler(zend_execute_data *execute_data);
char* dontbug_xdebug_cmd(char* command);

#endif
