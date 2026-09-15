import { Modal, Pressable, SafeAreaView, ScrollView, StyleSheet, Text, View } from "react-native";
import { LegalDoc, legalBodies, legalTitles } from "../legal/content";
import { colors } from "../theme";

export function LegalModal({ doc, visible, onClose }: { doc: LegalDoc | null; visible: boolean; onClose: () => void }) {
  if (!doc) return null;
  return (
    <Modal visible={visible} animationType="slide" presentationStyle="pageSheet" onRequestClose={onClose}>
      <SafeAreaView style={styles.safe}>
        <View style={styles.bar}>
          <Text style={styles.title}>{legalTitles[doc]}</Text>
          <Pressable onPress={onClose} hitSlop={12}>
            <Text style={styles.close}>Fechar</Text>
          </Pressable>
        </View>
        <ScrollView contentContainerStyle={styles.body}>
          <Text style={styles.text}>{legalBodies[doc]}</Text>
        </ScrollView>
      </SafeAreaView>
    </Modal>
  );
}

const styles = StyleSheet.create({
  safe: { flex: 1, backgroundColor: colors.canvas },
  bar: { flexDirection: "row", alignItems: "center", justifyContent: "space-between", paddingHorizontal: 20, paddingVertical: 14, borderBottomWidth: 1, borderBottomColor: colors.border },
  title: { fontSize: 17, fontWeight: "800", color: colors.navy },
  close: { color: colors.blue, fontWeight: "700", fontSize: 15 },
  body: { padding: 20, paddingBottom: 40 },
  text: { fontSize: 14, lineHeight: 22, color: "#344054" },
});
