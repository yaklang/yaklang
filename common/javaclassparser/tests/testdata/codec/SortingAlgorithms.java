package codec;

/**
 * SortingAlgorithms - a battery for nested-loop + array-mutation + recursion structuring. Each sorter
 * is a well-known algorithm whose CFG stresses a different shape the loop structurer must preserve:
 *   - bubbleSort: nested for/for with a length-1-i bound and an in-body swap (bastore/iastore)
 *   - insertionSort: while with a SHORT-CIRCUIT compound guard (j >= 0 && a[j] > key)
 *   - selectionSort: nested for/for tracking a min index, swap after the inner loop
 *   - quickSort: recursion + three stacked while loops (Hoare partition), inclusive bounds
 *   - heapSort / siftDown: build-heap loop + extract loop + a while with early `return` mid-body
 * main() sorts the same inputs five ways and folds each result into an FNV-1a fingerprint; the five
 * fingerprints must agree (cross-checks the oracle) and the decompiled round-trip must reproduce them
 * byte-for-byte (cross-checks the decompiler). Empty / singleton / duplicate / reverse / negative
 * inputs exercise the boundary branches.
 */
public class SortingAlgorithms {

    static int[] bubbleSort(int[] in) {
        int[] a = in.clone();
        for (int i = 0; i < a.length - 1; i++) {
            for (int j = 0; j < a.length - 1 - i; j++) {
                if (a[j] > a[j + 1]) {
                    int t = a[j];
                    a[j] = a[j + 1];
                    a[j + 1] = t;
                }
            }
        }
        return a;
    }

    static int[] insertionSort(int[] in) {
        int[] a = in.clone();
        for (int i = 1; i < a.length; i++) {
            int key = a[i];
            int j = i - 1;
            while (j >= 0 && a[j] > key) {
                a[j + 1] = a[j];
                j--;
            }
            a[j + 1] = key;
        }
        return a;
    }

    static int[] selectionSort(int[] in) {
        int[] a = in.clone();
        for (int i = 0; i < a.length - 1; i++) {
            int min = i;
            for (int j = i + 1; j < a.length; j++) {
                if (a[j] < a[min]) {
                    min = j;
                }
            }
            int t = a[min];
            a[min] = a[i];
            a[i] = t;
        }
        return a;
    }

    static void quickSort(int[] a, int lo, int hi) {
        if (lo >= hi) {
            return;
        }
        int pivot = a[(lo + hi) >>> 1];
        int i = lo;
        int j = hi;
        while (i <= j) {
            while (a[i] < pivot) {
                i++;
            }
            while (a[j] > pivot) {
                j--;
            }
            if (i <= j) {
                int t = a[i];
                a[i] = a[j];
                a[j] = t;
                i++;
                j--;
            }
        }
        quickSort(a, lo, j);
        quickSort(a, i, hi);
    }

    static int[] heapSort(int[] in) {
        int[] a = in.clone();
        int n = a.length;
        for (int i = n / 2 - 1; i >= 0; i--) {
            siftDown(a, i, n);
        }
        for (int end = n - 1; end > 0; end--) {
            int t = a[0];
            a[0] = a[end];
            a[end] = t;
            siftDown(a, 0, end);
        }
        return a;
    }

    static void siftDown(int[] a, int start, int end) {
        int root = start;
        while (root * 2 + 1 < end) {
            int child = root * 2 + 1;
            if (child + 1 < end && a[child] < a[child + 1]) {
                child++;
            }
            if (a[root] < a[child]) {
                int t = a[root];
                a[root] = a[child];
                a[child] = t;
                root = child;
            } else {
                return;
            }
        }
    }

    static String fingerprint(int[] a) {
        StringBuilder sb = new StringBuilder();
        long checksum = 1469598103934665603L;
        for (int i = 0; i < a.length; i++) {
            sb.append(a[i]);
            sb.append(i + 1 < a.length ? "," : "");
            checksum ^= (a[i] & 0xffffffffL);
            checksum *= 1099511628211L;
        }
        sb.append("#");
        sb.append(Long.toHexString(checksum));
        return sb.toString();
    }

    public static void main(String[] args) {
        int[][] inputs = {
            {5, 2, 9, 1, 5, 6},
            {3, 3, 3, 3},
            {1},
            {},
            {9, 8, 7, 6, 5, 4, 3, 2, 1, 0},
            {-4, 10, -7, 0, 3, -1, 8}
        };
        StringBuilder sb = new StringBuilder();
        for (int i = 0; i < inputs.length; i++) {
            int[] in = inputs[i];
            sb.append("bubble:").append(fingerprint(bubbleSort(in))).append(";");
            sb.append("insert:").append(fingerprint(insertionSort(in))).append(";");
            sb.append("select:").append(fingerprint(selectionSort(in))).append(";");
            int[] q = in.clone();
            quickSort(q, 0, q.length - 1);
            sb.append("quick:").append(fingerprint(q)).append(";");
            sb.append("heap:").append(fingerprint(heapSort(in))).append(";");
            sb.append("|");
        }
        System.out.println(sb);
    }
}
