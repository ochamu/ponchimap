// Package importer takes what an investigation found, and what the user adds
// to it, into the configuration model. A second investigation finds the
// elements of the first one again and leaves them under the IDs they have,
// so that the labels, groups, and hidden entries of a view config stay on
// the same elements. The rules follow, and the code names them by their
// labels and headings.
//
// A findings file is one acquisition: one read, named by acquisition.id, of a
// source, named by acquisition.source. A read that repeats a query gets a new
// ID and keeps the source. An acquisition is complete when it states all that
// its source says now, and acquisitions are older or newer by checked_at,
// then by ID. The nodes, edges, evidence and coverage of a findings file are
// candidates, whose IDs mean something only inside the file. Evidence and
// coverage are written by hand when they have no acquisition_id, or one that
// the records do not hold.
//
// # Identity
//
// A description is what a node candidate or a node of the model says: kind,
// provider, service, effective environment, and the native_id, logical_name,
// account_id and project_id of its identity. The effective environment of a
// candidate is its environment, else the target.environment of its
// acquisition, else that of the model. That of a node or an edge of the model
// is its environment, else that of the model. A description has the main key
// of the first tier it fits, and key N is the main key of tier N:
//
//  1. kind, provider, service, account_id, project_id, native_id: a native_id
//     and an owner, which is an account_id or a project_id.
//  2. kind, provider, service, effective environment, native_id: a native_id.
//  3. kind, provider, service, effective environment, account_id, project_id,
//     logical_name: a logical_name.
//  4. provider, service: the kind service.
//
// The matching applies these rules:
//
//   - N1: Descriptions with the same main key are one node.
//   - N2: A description of key 2 is the node of key 1 with the same kind,
//     provider, service and native_id that is known in its environment, when
//     exactly one such node fits.
//   - N3: A description of key 3 is the node of another key with the same
//     kind, provider and service that is known in its environment under its
//     logical_name and carries every owner that the description writes, when
//     exactly one such node fits. Descriptions that write more of the owner
//     are settled first, and each counts the nodes as those before it left
//     them.
//   - N4: When N2 or N3 finds several nodes, the description stays with the
//     node it was taken as before, if that node is among them. Otherwise it
//     joins none, is a node by its own key, and warns
//     import.ambiguous_identity.
//   - N5: Import never merges nodes of the model. When N1 to N3 make several
//     of them one, a candidate goes to the node it was taken as before, else
//     to a node that has its key, else to the node it joined by N2 or N3,
//     else to the first in the model. Each later one warns
//     import.duplicate_identity.
//
// A node is known in an environment and under a name by what states it now:
// the candidates of the run, and what the records hold of the acquisitions
// that stay in the model and are not given again. A node with no evidence,
// or with evidence that no record accounts for, is stated by its own identity
// too. Only when no node fits by these do the identity and the environment of
// the other nodes count. What the
// records hold is matched again only when its acquisition is given again. A
// description of key 3 that joins a node does not make that node known. A
// node that leaves the model in the run takes no part.
//
// A node of the model keeps its ID and environment, and its identity gains
// what the candidates add. Candidates alone are a new node, which writes the
// environment when theirs agree. A field that a candidate gives another value
// keeps the value of the node, or of the first candidate in Order, and warns
// import.identity_conflict.
//
// # IDs
//
// newID makes a new ID from the kind of ID and these values:
//
//   - node, n-: the fields of the least main key of its descriptions.
//   - edge, e-: from, to, relation, effective environment, slot.
//   - evidence, ev-: the element ID, kind, supports, ref.type, ref.path and
//     valid_time, but not the lines, checked_at or acquisition ID.
//   - coverage, cv-: the source of its acquisition and its scope as matched.
//   - supplements, sp-: the value of the file. The same value keeps the
//     acquisition ID it was recorded under.
//
// A node, edge, evidence or coverage ID that is taken is made again with a
// number, 1, 2 and so on, added to the values. New nodes and edges avoid
// every ID of the model as read, and new evidence and coverage the IDs of the
// model to be saved. A new evidence item or coverage record first takes over
// the ID and the place of one that the run took out and that has the same
// values: one equal in all but the ID, then the others in Order.
//
// # Replacing
//
// Evidence and coverage that are taken in carry the ID of their acquisition
// in acquisition_id. An acquisition given again replaces what it gave before.
//
//   - M1: Of the acquisitions of a source, those older than the newest
//     complete one leave the model with their evidence and coverage, and
//     their candidates take no part.
//   - M2: Evidence of an acquisition that stays leaves when newer acquisitions
//     of its source restate it: their evidence on the same element, of the
//     same kind and valid_time, states every support key of it between them.
//     Each edge of a slot is an element of its own.
//   - M3: The coverage of the newest acquisition of a source stays. That of
//     an older one stays while its status is complete and evidence of its
//     acquisition stays in the model.
//   - M4: M1 to M3 never take out evidence or coverage written by hand.
//   - M5: The supplements are a source of their own. A supplements file that
//     is given replaces all evidence of earlier supplements, whatever its
//     time, and a run without the file keeps it. Each statement is one stated
//     evidence item, whose ref is the file and the lines of the statement, on
//     the model as R1 to R3 leave it. A statement that names an ID the model
//     lacks, or a connection.target that is no node of it, is left out and
//     warns import.unknown_target.
//   - M6: Evidence keeps its ref, checked_at and valid_time as given. Nothing
//     that is replaced is kept with a valid_time added.
//
// # Removing
//
//   - R1: An edge that lost evidence in the run, and has none left apart from
//     that of the supplements, leaves the model and warns import.removed.
//   - R2: So does a node.
//   - R3: An element that R1 or R2 would remove stays, with the evidence it
//     would have lost, while something names it: an end of an edge that
//     stays, the connection.target of evidence that stays and is not of the
//     supplements, or the target_ids of the target scope of the model. It
//     warns import.kept_superseded on every run while it stays so.
//   - R4: Coverage loses from its target_ids the elements that left, and a
//     record left with none leaves, since one without target_ids would cover
//     more. This holds for coverage written by hand too.
//   - R5: A new node or edge that no candidate gives evidence is an error,
//     import.no_evidence. A candidate without evidence only names an element
//     that the model has.
//
// # Attributes
//
// Only an element that gained or lost evidence in the run, or that a
// candidate was taken as, is settled again. The others keep what a person
// wrote.
//
//   - A1: The role and the region of a node follow its evidence that stays and
//     has no valid_time. One value is written. Two or more remove the key,
//     warn import.attribute_disputed, and leave the values of candidates
//     unused. With none, the value is removed only when evidence that left in
//     the run stated it, and A2 applies.
//   - A2: A candidate writes a key that A1 and A3 did not settle when the
//     element lacks it, or when its value is what an older acquisition of the
//     candidate's source wrote there. What another source wrote stays.
//     Candidates write one by one in Order.
//   - A3: When the evidence of the supplements without a valid_time states one
//     role, that role is written in place of A1, without a warning.
//
// # Order
//
// A run takes the acquisitions oldest first, and the candidates of one
// acquisition in the order of their IDs, never in the order of the files or
// of the candidates in a file. The model keeps the order of the nodes, edges,
// coverage and evidence it had, and new ones follow in the order of their
// IDs. So one run saves the same bytes whatever the order of its inputs, and
// the same inputs again change nothing. The evidence that stays does not
// depend on how the findings are split into runs, apart from the
// supplements, of which the last file given stays.
//
// # Reading
//
// Each input is read in stages: its text, by input.DecodeJSON or
// input.DecodeYAML; the embedded schema; for findings, the IDs and references
// of the candidates; and last, the acquisition IDs, which no two inputs share
// and none carries with a content other than recorded. Matching starts only
// when no input has an error, and otherwise nothing is saved. A diagnostic
// names a candidate by its file and JSON Pointer, never by its ID.
//
// # Records and saving
//
// The workspace keeps acquisitions.json, the records: of each source the
// newest acquisition and the newest complete one, so that an older input
// given later still leaves by M1, every acquisition that the model names, and
// the supplements. While an acquisition and an element stay in the model, its
// record also keeps what its candidates said of the element, which the
// matching and A2 read. Without the records, all evidence counts as written
// by hand.
//
// A run works everything out in memory, Check included. Then it writes the
// records to acquisitions.json.next, writes the model unless its bytes are the
// same, and renames acquisitions.json.next to acquisitions.json, so that the
// records never run ahead of the model or behind it. Two runs at once are not
// supported.
package importer
