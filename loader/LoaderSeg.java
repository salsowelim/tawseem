import com.opencsv.CSVReader;
import java.sql.Statement;
import java.util.ArrayList;
import java.io.FileReader;
import java.sql.Connection;
import java.sql.DriverManager;
import java.sql.PreparedStatement;
import java.io.BufferedReader;
import java.io.FileReader;
import java.io.IOException;
import java.util.Scanner;
import java.io.File;
import java.util.Arrays;
import java.util.regex.Pattern;
import java.util.regex.Matcher;
import java.sql.ResultSet;

public class LoaderSeg {
    public static void main(String[] args) throws Exception {
        Class.forName("org.sqlite.JDBC");
        if (args.length != 2) {
            System.out.println("Please provide only those two arguments (in order):\n 1.sqlite file path (if not exist, will create new one) \n 2.path to data (text files) directory\n ");
            System.exit(1);
        }
        String db_path = args[0];
        String data_dir_path = args[1];
        Connection conn = DriverManager.getConnection("jdbc:sqlite:" + db_path);
        conn.setAutoCommit(false);
        Statement stat = conn.createStatement();
        stat.execute("PRAGMA foreign_keys = ON;");
        if (new File(db_path).length() <= 0) {
            // create database if file not exists - create dummy user
            // text processed flag: 0 unprocessed, 1 processed
            stat.execute("CREATE TABLE texts (t_id INTEGER PRIMARY KEY , content TEXT, processed INTEGER)");
            stat.execute("CREATE TABLE users (u_id INTEGER PRIMARY KEY , username TEXT)");
            // word processed flag: 0 unprocessed, 1 processed, 2 pre-processed
            stat.execute("CREATE TABLE words (w_seq INTEGER NOT NULL,  text_id INTEGER NOT NULL, word TEXT, pr1 TEXT, pr2 TEXT, pr3 TEXT, pr4 TEXT,stm TEXT, sf1 TEXT, sf2 TEXT, sf3 TEXT, sf4 TEXT, processed INTEGER, FOREIGN KEY(text_id) REFERENCES texts(t_id),PRIMARY KEY (w_seq, text_id))");
            stat.execute("CREATE TABLE workson (user_id INTEGER NOT NULL, text_id INTEGER NOT NULL, FOREIGN KEY (user_id) REFERENCES users(u_id), FOREIGN KEY (text_id) REFERENCES texts(t_id), PRIMARY KEY (user_id, text_id))");
            conn.commit();
            stat.execute("INSERT INTO users (u_id,username) VALUES (1,\'user1\')");
            conn.commit();
        }
        conn.setAutoCommit(true);
        stat.close();
        String[] words = null;
        ArrayList<String[]> pre_seg_list = new ArrayList<String[]>();
        String largeText = "";
        // loading the pre-processed list (or white-flag list)
        try {
            CSVReader reader = new CSVReader(new FileReader(data_dir_path + "/pre_proccesed_list.csv"), ',' , '"' , 0);
            String[] nextLine;
            while ((nextLine = reader.readNext()) != null) {
                if (nextLine != null) {
                    pre_seg_list.add(nextLine);
                }
            }
        } catch (IOException e) {
            e.printStackTrace();
        }

        File dir = new File(data_dir_path);
        File[] directoryListing = dir.listFiles();
        for (File child : directoryListing) {
            PreparedStatement prep = conn.prepareStatement("INSERT INTO texts (t_id, content, processed) VALUES (?,?, ?)");
            PreparedStatement prep2 = conn.prepareStatement("INSERT OR IGNORE INTO words (w_seq, word,pr1,pr2,pr3,pr4,stm,sf1,sf2,sf3,sf4,processed,text_id) VALUES (?, ?, ?,?,?,?,?,?,?,?,?,?,?)");
            if (child.getName().contains("DS_Store") || child.getName().contains(".csv")) {
                continue;
            }
            int t_id = Integer.parseInt(child.getName().substring(0, child.getName().lastIndexOf('.')));
            BufferedReader br = new BufferedReader(new FileReader(child));
            try {
                StringBuilder sb = new StringBuilder();
                String line = br.readLine();
                while (line != null) {
                    sb.append(line);
                    sb.append("\n");
                    line = br.readLine();
                }
                largeText = sb.toString();
            } catch (Exception e) {
                System.err.println("Error: " + e.getMessage());
            } finally {
                br.close();
            }

            // add text to db
            prep.setInt(1, t_id);
            prep.setString(2, largeText);
            prep.setInt(3, 0);
            prep.addBatch();
            try {
                words = largeText.split("[\\t\\n\\v\\f\\r\\s\\u0085\\u00A0\\u0020\\u202C]+");
            } catch (Exception e) {
                System.err.println("Error: " + e.getMessage());
                continue;
            }

            // add words to db
            int i;
            int actual_i = -1;
            for (i = 0 ; i < words.length; i++) {
                if (words[i].length() == 0) {
                    System.out.println(words[i] + " is zero length !!");
                    continue;
                }
                actual_i++;
                boolean word_is_pre_segmented = false;
                // segmented flag: 0 is not segmented. 1 is segmented. 2 presegmented
                for (int j = 0 ; j < pre_seg_list.size(); j++) {
                    if (pre_seg_list.get(j)[0].trim().equals(words[i].trim())) {
                        //thrword,pr1,pr2,pr3,pr4,sf1,sf2,sf3,sf4,stm
                        word_is_pre_segmented = true;
                        prep2.setInt(1, actual_i + 1);
                        prep2.setString(2, pre_seg_list.get(j)[0]);
                        setStringOrNull(prep2, 3, pre_seg_list.get(j)[1]);
                        setStringOrNull(prep2, 4, pre_seg_list.get(j)[2]);
                        setStringOrNull(prep2, 5, pre_seg_list.get(j)[3]);
                        setStringOrNull(prep2, 6, pre_seg_list.get(j)[4]);
                        setStringOrNull(prep2, 7, pre_seg_list.get(j)[9]);
                        setStringOrNull(prep2, 8, pre_seg_list.get(j)[5]);
                        setStringOrNull(prep2, 9, pre_seg_list.get(j)[6]);
                        setStringOrNull(prep2, 10, pre_seg_list.get(j)[7]);
                        setStringOrNull(prep2, 11, pre_seg_list.get(j)[8]);
                        prep2.setInt(12, 2);
                        prep2.setInt(13, t_id);
                        prep2.addBatch();
                        break;
                    }
                }
                if (!word_is_pre_segmented) {
                    Pattern p1 = Pattern.compile("^[a-zA-Z0-9_.-]+$");
                    Matcher m1 = p1.matcher(words[i]);
                    Pattern p2 = Pattern.compile("^[\\u0660-\\u0669\\u066A\\u066B\\u066C\\u066D\\u061F\\u060C]+$");
                    Matcher m2 = p2.matcher(words[i]);
                    Pattern p3 = Pattern.compile("^[\"\\\\§¯·×Ø•✽●◄™'{}#$,_.,+*)(&%$#!”“’:;%^‘❊ﭙÀÁÂÃÄÅÆÇÈÉÊËÌÍÎÏàáâãäåæçèéêëìíîïÐÑÒÓÔÕÖØÙÚÛÜÝÞßðñòóôõöøùúûüýþÿ]+$");
                    Matcher m3 = p3.matcher(words[i]);
                    if (m1.matches() || m2.matches() || m3.matches() ) {
                        //match expressions, skip
                        prep2.setInt(1, actual_i + 1);
                        prep2.setString(2, words[i]);
                        prep2.setString(7, words[i]);
                        prep2.setNull(3, java.sql.Types.VARCHAR);
                        prep2.setNull(4, java.sql.Types.VARCHAR);
                        prep2.setNull(5, java.sql.Types.VARCHAR);
                        prep2.setNull(6, java.sql.Types.VARCHAR);
                        prep2.setNull(8, java.sql.Types.VARCHAR);
                        prep2.setNull(9, java.sql.Types.VARCHAR);
                        prep2.setNull(10, java.sql.Types.VARCHAR);
                        prep2.setNull(11, java.sql.Types.VARCHAR);
                        prep2.setInt(12, 2);
                        prep2.setInt(13, t_id);
                        prep2.addBatch();
                    } else {
                        // add normal word
                        prep2.setInt(1, actual_i + 1);
                        prep2.setString(2, words[i]);
                        prep2.setNull(3, java.sql.Types.VARCHAR);
                        prep2.setNull(4, java.sql.Types.VARCHAR);
                        prep2.setNull(5, java.sql.Types.VARCHAR);
                        prep2.setNull(6, java.sql.Types.VARCHAR);
                        prep2.setNull(7, java.sql.Types.VARCHAR);
                        prep2.setNull(8, java.sql.Types.VARCHAR);
                        prep2.setNull(9, java.sql.Types.VARCHAR);
                        prep2.setNull(10, java.sql.Types.VARCHAR);
                        prep2.setNull(11, java.sql.Types.VARCHAR);
                        prep2.setInt(12, 0);
                        prep2.setInt(13, t_id);
                        prep2.addBatch();
                    }
                }
            }
            prep.executeBatch();
            prep2.executeBatch();
        }
    }
    private static void setStringOrNull(PreparedStatement pstmt, int column, String value) throws Exception {
        if (value != null && !value.isEmpty()) {
            pstmt.setString(column, value);
        } else {
            pstmt.setNull(column, java.sql.Types.VARCHAR);
        }
    }

}
