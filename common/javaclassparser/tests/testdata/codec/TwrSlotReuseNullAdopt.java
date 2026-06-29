package codec;

import java.util.ArrayList;
import java.util.Collections;
import java.util.Iterator;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.Scanner;

// Reproduces Bug AL's null-adopt-once case (commons-codec DaitchMokotoffSoundex.<clinit>): one JVM
// slot is reused for the verbose try-with-resources synthetic `Throwable primaryExc = null` AND a
// later `Map.Entry e` loop variable whose live ranges are disjoint. The decompiler models slots with
// a single global table and lets a null-initialized ref adopt a concrete type; without committing the
// adoption the same ref adopts a SECOND, incompatible type, merging the two distinct variables onto
// one mistyped declaration (`Map.Entry var = null`) so `var = <throwable>` and `var.addSuppressed(..)`
// fail to recompile. The fix (AssignVarGuarded null-adopt-once, JavaRef.nullTypeAdopted) keeps
// primaryExc typed Throwable and mints a fresh local for the loop variable.
//
// Modern javac only reuses the slot when primaryExc/scanner go OUT OF SCOPE before the loop, so the
// twr desugaring is hand-written inside a nested block (releasing slots 0/1) and the loop variable is
// used twice (forcing it into a slot rather than folding inline) — this faithfully recreates the
// old-javac bytecode shape with a current JDK.
public class TwrSlotReuseNullAdopt {
    static final Map<Character, List<String>> RULES = new LinkedHashMap<Character, List<String>>();
    static final StringBuilder OUT = new StringBuilder();

    static void fill(Scanner sc) {
        while (sc.hasNextLine()) {
            String line = sc.nextLine();
            char k = line.charAt(0);
            List<String> lst = RULES.get(k);
            if (lst == null) {
                lst = new ArrayList<String>();
                RULES.put(k, lst);
            }
            lst.add(line);
        }
    }

    static void load() {
        {
            Scanner sc = new Scanner("b yy\nb xx\na zz\n");
            Throwable primaryExc = null;
            try {
                fill(sc);
            } catch (Throwable t) {
                primaryExc = t;
                throw t;
            } finally {
                if (sc != null) {
                    if (primaryExc != null) {
                        try {
                            sc.close();
                        } catch (Throwable t2) {
                            primaryExc.addSuppressed(t2);
                        }
                    } else {
                        sc.close();
                    }
                }
            }
        }
        Iterator<Map.Entry<Character, List<String>>> it = RULES.entrySet().iterator();
        while (it.hasNext()) {
            Map.Entry<Character, List<String>> e = it.next();
            List<String> v = e.getValue();
            Collections.sort(v);
            OUT.append(e.getKey()).append('=').append(v).append(';');
        }
    }

    public static void main(String[] args) {
        load();
        System.out.println(OUT.toString());
    }
}
