import CommercialFlow from "./src/app/CommercialFlow";

const isDemo = process.env.EXPO_PUBLIC_DEMO_MODE === "true";

export default function App() {
  return <CommercialFlow demo={isDemo} />;
}
