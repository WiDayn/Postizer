# Frontend Methodology

Postizer's frontend should be built from a small, explicit component library before page-specific views are assembled. The black-and-white newspaper style is a visual language, not an excuse for inconsistent controls.

## Order Of Work

1. Define tokens:
   - Colors, fonts, borders, spacing, shell width, and state colors.
   - Use black, white, and grayscale for the default theme.
   - Tokens live in `:root` in `web/static/site.css`.

2. Define primitives:
   - Buttons, links that behave like buttons, fields, check rows, status badges, panels, toolbars, separators, lists, and media buttons.
   - Primitive classes use the `ui-*` prefix.

3. Define composites:
   - Newspaper masthead, section nav, front page grid, editor topbar, post library, editor stage, document sidebar, media strip.
   - Composite classes may use domain names such as `editor-*`, but should be built from primitives.

4. Compose pages:
   - Templates should prefer component classes over one-off CSS.
   - Page-specific classes should describe layout or domain behavior only.

5. Verify:
   - Check desktop and mobile layouts.
   - Confirm no button text overflows.
   - Keep public pages server-rendered and light.
   - Run `go test ./...` after template or server changes.

## Base Component Library

Current primitives:

- `ui-button`: standard command button.
- `ui-button--primary`: primary action.
- `ui-button--ghost`: link-like secondary command.
- `ui-status`: compact state indicator.
- `ui-panel`: bordered grouped surface.
- `ui-panel__head`: panel header with title and optional action/count.
- `ui-field`: label plus input/textarea.
- `ui-check`: checkbox row.
- `ui-input`: text/date/file input.
- `ui-textarea`: textarea.
- `ui-toolbar`: compact command toolbar.
- `ui-separator`: toolbar separator.
- `ui-list-button`: dense selectable list row.
- `ui-media-button`: square image insertion button.

## Naming Rule

- `ui-*`: reusable primitives.
- `editor-*`: editor-specific layout and behavior.
- `newspaper-*`, `front-*`, `section-*`: public newspaper shell and front page composites.
- `article-*`, `post-*`, `tag-*`: public content views.
- Avoid styling new controls directly with IDs.

## Design Rule

If a new view needs a control that already exists conceptually, extend the primitive with a modifier class instead of inventing a new visual pattern.
