# Built-in icons

These are the icons a card shows next to a service name. Each file is one
category that a findings file can give a node in `attributes.category`, and
`generic.svg` is the fallback for a node that gives none.

## Origin and license

Every icon here is an original work drawn for this project from basic
geometric shapes. They are covered by the repository license (Apache-2.0).
None is copied or traced from a vendor logo or from another icon set.

The figure uses only these icons. It does not use vendor logos or vendor
icon packs, and it cannot load them. The service name and the account or
project box show whose product a card is. A logo could cover only some of the
services an app uses, and each vendor sets its own terms for its logos. The
built-in set shows what kind of service a card is.

## Visual system

- Canvas: `viewBox="0 0 48 48"`.
- Tile: `<rect x="2" y="2" width="44" height="44" rx="10">` filled with the
  family color.
- Glyph: white, inside the 8..40 area. Strokes are 3 units wide with round
  caps and joins. Solid parts are filled white.
- Color by family, each at least 4.5:1 against white:

| Family | Color | Categories |
| --- | --- | --- |
| Execution | `#c2410c` | compute, pipeline |
| Data | `#1d4ed8` | database, storage, keyvalue |
| Messaging | `#7e22ce` | queue, message, mail |
| Web and integration | `#0f766e` | hosting, network, api |
| Security and operations | `#be123c` | auth, monitor |
| Commerce | `#15803d` | payment |
| Client | `#334155` | client |
| Fallback | `#64748b` | generic |

## A new category

The tool knows no service. A findings file gives each node the category that
says what the service does for the app, such as `database` for Cloudflare D1.
Add a new category only when none of the existing ones fits. A new category
needs its own icon here and a row in the family table.

## Rules for a new icon

The file name is the icon key: `Icon("queue")` reads `queue.svg`. Add the
file here, the enum of `attributes.category` in `schema/model.schema.json`,
and its row in the category table of `skills/ponchimap/references/findings.md`. A test checks that the
enum names every file here but `generic.svg`.

`icons_test.go` checks these rules:

- Standalone SVG with `xmlns="http://www.w3.org/2000/svg"` and
  `viewBox="0 0 48 48"`.
- Only `svg`, `g`, `rect`, `circle`, `ellipse`, `line`, `polyline`,
  `polygon`, and `path`. This excludes `script`, `style`, `text`, `use`,
  `image`, gradients, and filters.
- No `href` attribute and no `url()` value, so nothing outside the file is
  referenced.
- No text content. Fonts are not available to icons.
- At most 1536 bytes.
- Not byte-identical to another icon.

The tests do not check these, so check them by rendering a diagram to PNG:

- The PNG rasterizer imports SVG with a restricted feature set, and one icon
  it rejects fails the whole PNG render. Style with presentation attributes
  such as `fill` and `stroke`.
- Draw a dot as a `circle`. A zero-length stroked subpath with a round or
  square cap, such as `M24 24h0`, is rejected.
- The icon is drawn at 60x60 layout units on a white card, and a large figure
  fitted to a page shows it much smaller. Keep the glyph readable when small
  and distinct from the other categories.
