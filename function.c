#include <stdio.h>
#include <stdlib.h>
#include <errno.h>
#include <libgen.h>
#include <string.h>
#include <errno.h>

#include <clang-c/Index.h>
#include <clang-c/Platform.h>

#include "function.h"


enum CXChildVisitResult cursorVisitor(CXCursor cursor, CXCursor parent, CXClientData client_data);

typedef struct CData {
	functions_array* fa;
	const char* fname;
} CData;

functions_array* fa_new(size_t cap)
{
	functions_array* fa = malloc(sizeof(functions_array));
	fa->len = 0;
	fa->cap = cap;
	fa->data = malloc(fa->cap * sizeof(void*));

	return fa;
}

void fa_add(functions_array* fa, function* f)
{
	if (fa->len >= fa->cap) {
		fa->cap <<= 1; // grow exponentially
		fa->data = realloc(fa->data, fa->cap * sizeof(void*));
	}
	fa->data[fa->len] = f;
	fa->len++;
}

void fa_free(functions_array* fa) {
	size_t i;
	for (i = 0; i < fa->len; i++) {
		free(fa->data[i]);
	}
	free(fa->data);
	free(fa);
}

function* fa_at(functions_array* fa, size_t i) {
	return fa->data[i];
}


functions_array* get_functions(const char* fname, const char* contents,
		unsigned long contents_len)
{
	/*printf("DEBUG:\nfname: %s\ncontents: %s\nlen: %lu\n", fname, contents, contents_len);*/

	struct CXUnsavedFile *unsaved = NULL;
	functions_array* fa;
	int unsaved_cnt = (contents_len == 0) ? 0 : 1;
	CXIndex index = clang_createIndex(0, 0);
	CData cdata;

	if(index == 0) {
		errno = EIO;
		return NULL;
	}

	/* build unsaved file */
	if (contents_len != 0) {
		unsaved = malloc(sizeof(struct CXUnsavedFile));
		unsaved->Filename = fname;
		unsaved->Contents = contents;
		unsaved->Length = contents_len;
	}

	CXTranslationUnit translationUnit = clang_parseTranslationUnit(index,
			fname, 0, 0, unsaved, unsaved_cnt,
			CXTranslationUnit_Incomplete | CXTranslationUnit_DetailedPreprocessingRecord);

	if (translationUnit == 0) {
		errno = ENOENT;
		return NULL;
	}

	/* get diagnostics */
	/*
	CXDiagnosticSet diags = clang_getDiagnosticSetFromTU(translationUnit);
	for (unsigned i = 0; i < clang_getNumDiagnosticsInSet(diags); i++) {
		CXDiagnostic d = clang_getDiagnosticInSet(diags, i);
		CXString str = clang_formatDiagnostic(d, clang_defaultDiagnosticDisplayOptions());
		puts(clang_getCString(str));
		clang_disposeString(str);
		clang_disposeDiagnostic(d);
	}
	clang_disposeDiagnosticSet(diags);
	*/


	CXCursor rootCursor = clang_getTranslationUnitCursor(translationUnit);


	fa = fa_new(10);
	cdata.fa = fa;
	cdata.fname = fname;
	clang_visitChildren(rootCursor, *cursorVisitor, (CXClientData)&cdata);

	clang_disposeTranslationUnit(translationUnit);
	clang_disposeIndex(index);
	if (unsaved != NULL) {
		free(unsaved);
	}
	return fa;
}


enum CXChildVisitResult cursorVisitor(CXCursor cursor, CXCursor parent, CXClientData client_data)
{
	enum CXCursorKind kind = clang_getCursorKind(cursor);
	CXString name = clang_getCursorSpelling(cursor);

	if (kind == CXCursor_FunctionDecl || kind == CXCursor_ObjCInstanceMethodDecl) {
		CXSourceRange extent = clang_getCursorExtent(cursor);
		CXSourceLocation start = clang_getRangeStart(extent);
		CXSourceLocation end = clang_getRangeEnd(extent);

		CXFile loc_file;
		CData* cdata = (CData*)client_data;
		clang_getExpansionLocation(start, &loc_file, NULL, NULL, NULL);
		CXString cx_loc_fname = clang_getFileName(loc_file);

		/*printf("file %s\nfunc %s\n", cdata->fname, clang_getCString(cx_loc_fname));*/
		if (strcmp(cdata->fname, clang_getCString(cx_loc_fname)) == 0) { // not a local function
			function* f = malloc(sizeof(function));
			f->name = clang_getCString(name);
			clang_getExpansionLocation(start, NULL, &(f->start_line), NULL, NULL);
			clang_getExpansionLocation(end, NULL, &(f->end_line), NULL, NULL);

			fa_add(cdata->fa, f);
		}
		clang_disposeString(cx_loc_fname);

		return CXChildVisit_Continue;
	}
	return CXChildVisit_Recurse;
}

